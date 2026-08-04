package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestStripTerminalResponses(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "no sequences",
			input: []byte("hello world\r\n$ "),
			want:  []byte("hello world\r\n$ "),
		},
		{
			name:  "empty input",
			input: []byte{},
			want:  []byte{},
		},
		{
			name:  "OSC 11 response with ESC backslash",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\"),
			want:  []byte{},
		},
		{
			name:  "OSC 11 response with BEL",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x07"),
			want:  []byte{},
		},
		{
			name:  "DA1 response",
			input: []byte("\x1b[?1;2c"),
			want:  []byte{},
		},
		{
			name:  "DA1 response with multiple params",
			input: []byte("\x1b[?64;1;2;6;22c"),
			want:  []byte{},
		},
		{
			name:  "CPR response row;col",
			input: []byte("\x1b[5;1R"),
			want:  []byte{},
		},
		{
			name:  "CPR response row only",
			input: []byte("\x1b[1R"),
			want:  []byte{},
		},
		{
			name:  "only responses produces empty",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\\x1b[?1;2c\x1b[5;1R"),
			want:  []byte{},
		},
		{
			name:  "mixed content preserves normal output",
			input: []byte("$ ls\r\nfile.txt\r\n\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[?1;2c$ "),
			want:  []byte("$ ls\r\nfile.txt\r\n$ "),
		},
		{
			name:  "sequences between normal text",
			input: []byte("before\x1b[24;80Rafter"),
			want:  []byte("beforeafter"),
		},
		{
			name:  "multiple OSC 11 responses",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\\x1b]11;rgb:ffff/ffff/ffff\x07"),
			want:  []byte{},
		},
		{
			name:  "preserves other escape sequences",
			input: []byte("\x1b[32mgreen\x1b[0m \x1b[?1;2c normal"),
			want:  []byte("\x1b[32mgreen\x1b[0m  normal"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTerminalResponses(tt.input)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("stripTerminalResponses(%q)\n got: %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTerminalRoute(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		wantKind string
		wantID   string
	}{
		{name: "environment route", target: "/environment/env-1", wantKind: terminalRouteEnvironment, wantID: "env-1"},
		{name: "session route", target: "/session/session-1", wantKind: terminalRouteSession, wantID: "session-1"},
		{name: "unknown route", target: "/unknown-target", wantKind: "", wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTerminalRoute(&gin.Context{Params: gin.Params{{Key: "target", Value: tt.target}}})
			if got.kind != tt.wantKind || got.id != tt.wantID {
				t.Fatalf("parseTerminalRoute() = {%q %q}, want {%q %q}",
					got.kind, got.id, tt.wantKind, tt.wantID)
			}
		})
	}
}

func TestSessionTerminalRouteRequiresAgentMode(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "missing mode", query: ""},
		{name: "shell mode", query: "?mode=shell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/terminal/session/session-1"+tt.query, nil)

			handler := &TerminalHandler{}
			handler.handleSessionTerminalRoute(c, "session-1")

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}

			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["error"] != "session terminal route requires mode=agent; shell terminals must use /terminal/environment/:environmentId" {
				t.Fatalf("error = %q", body["error"])
			}
		})
	}
}

func TestWaitForRemoteExecutionReadyRechecksReplacedExecution(t *testing.T) {
	log := testTerminalLogger(t)
	manager := lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
	handler := NewTerminalHandler(manager, nil, nil, log)

	staleServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer staleServer.Close()
	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer readyServer.Close()

	oldExecution := testRemoteExecution(t, staleServer.URL, "exec-old", "session-1", "env-1", log)
	newExecution := testRemoteExecution(t, readyServer.URL, "exec-new", "session-1", "env-1", log)

	if err := manager.ExecutionStoreForTesting().Add(oldExecution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		manager.RemoveExecution(oldExecution.ID)
		if err := manager.ExecutionStoreForTesting().Add(newExecution); err != nil {
			t.Errorf("add new execution: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, ok := handler.waitForRemoteExecutionReadyWithTimeout(ctx, "session-1", 1500*time.Millisecond)
	if !ok {
		t.Fatal("waitForRemoteExecutionReadyWithTimeout returned not ready")
	}
	if got.ID != newExecution.ID {
		t.Fatalf("execution ID = %q, want %q", got.ID, newExecution.ID)
	}
}

func testRemoteExecution(
	t *testing.T,
	serverURL string,
	executionID string,
	sessionID string,
	taskEnvironmentID string,
	log *logger.Logger,
) *lifecycle.AgentExecution {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, portString, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split server host/port: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	instance := &lifecycle.ExecutorInstance{
		InstanceID:    executionID,
		Client:        agentctlclient.NewClient(host, port, log),
		RuntimeName:   "docker",
		WorkspacePath: "/workspace",
	}
	return instance.ToAgentExecution(&lifecycle.ExecutorCreateRequest{
		TaskID:            "task-1",
		SessionID:         sessionID,
		TaskEnvironmentID: taskEnvironmentID,
		WorkspacePath:     "/workspace",
	})
}

func testTerminalLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: t.TempDir() + "/test.log",
	})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	return log
}

type stubExecutorProfileReader struct {
	session *taskmodels.TaskSession
	env     *taskmodels.TaskEnvironment
	profile *taskmodels.ExecutorProfile
}

func (s *stubExecutorProfileReader) GetTaskSession(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return s.session, nil
}

func (s *stubExecutorProfileReader) GetTaskEnvironment(_ context.Context, _ string) (*taskmodels.TaskEnvironment, error) {
	return s.env, nil
}

func (*stubExecutorProfileReader) HasActiveTaskResourceCleanupJob(context.Context, string) (bool, error) {
	return false, nil
}

func (s *stubExecutorProfileReader) GetExecutorProfile(_ context.Context, _ string) (*taskmodels.ExecutorProfile, error) {
	return s.profile, nil
}

// A shell terminal opened on a workspace must start with the executor profile's
// env vars exported — the agent subprocess and the repository setup script both
// get them, and the terminal was the remaining gap.
func TestStartUserShellProcessExportsExecutorProfileEnv(t *testing.T) {
	log := testTerminalLogger(t)
	manager := lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
	manager.SetExecutorProfileReader(&stubExecutorProfileReader{
		session: &taskmodels.TaskSession{ID: "session-1", ExecutorProfileID: "prof-1"},
		env:     &taskmodels.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
		profile: &taskmodels.ExecutorProfile{
			ID:      "prof-1",
			EnvVars: []taskmodels.ProfileEnvVar{{Key: "FONTAWESOME_NPM_AUTH_TOKEN", Value: "fa-secret-value"}},
		},
	})
	handler := NewTerminalHandler(manager, nil, nil, log)
	runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)
	t.Cleanup(func() {
		_ = runner.StopUserShell(context.Background(), "env-1", "term-1")
	})

	execution := &lifecycle.AgentExecution{
		ID:                "exec-1",
		SessionID:         "session-1",
		TaskEnvironmentID: "env-1",
		WorkspacePath:     t.TempDir(),
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal/environment/env-1?terminalId=term-1", nil)

	processID, status, errMsg := handler.startUserShellProcess(c, execution, "env-1", "term-1", runner)
	if errMsg != "" {
		t.Fatalf("startUserShellProcess() status=%d error=%q", status, errMsg)
	}

	env, ok := runner.StartEnvForTesting(processID)
	if !ok {
		t.Fatalf("process %q not registered", processID)
	}
	if env["FONTAWESOME_NPM_AUTH_TOKEN"] != "fa-secret-value" {
		t.Fatalf("shell env = %#v, want executor-profile var exported", env)
	}
}

func TestStartUserShellProcessExportsEffectiveRuntimeEnv(t *testing.T) {
	log := testTerminalLogger(t)
	manager := lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
	handler := NewTerminalHandler(manager, nil, nil, log)
	runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)
	t.Cleanup(func() {
		_ = runner.StopUserShell(context.Background(), "env-1", "term-1")
	})

	execution := (&lifecycle.ExecutorInstance{
		InstanceID:    "exec-1",
		WorkspacePath: t.TempDir(),
	}).ToAgentExecution(&lifecycle.ExecutorCreateRequest{
		SessionID:         "session-1",
		TaskEnvironmentID: "env-1",
		WorkspacePath:     t.TempDir(),
		Env: map[string]string{
			"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "http://127.0.0.1:9876",
			"GIT_CONFIG_COUNT":                    "1",
			"GIT_CONFIG_KEY_0":                    "credential.helper",
			"GIT_CONFIG_VALUE_0":                  "!agentctl git-credential",
			"PATH":                                "/tmp/kandev-shim:/usr/bin",
		},
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal/environment/env-1?terminalId=term-1", nil)

	processID, status, errMsg := handler.startUserShellProcess(c, execution, "env-1", "term-1", runner)
	if errMsg != "" {
		t.Fatalf("startUserShellProcess() status=%d error=%q", status, errMsg)
	}
	env, ok := runner.StartEnvForTesting(processID)
	if !ok {
		t.Fatalf("process %q not registered", processID)
	}
	for key, want := range map[string]string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "http://127.0.0.1:9876",
		"GIT_CONFIG_COUNT":                    "1",
		"GIT_CONFIG_KEY_0":                    "credential.helper",
		"GIT_CONFIG_VALUE_0":                  "!agentctl git-credential",
		"PATH":                                "/tmp/kandev-shim:/usr/bin",
	} {
		if env[key] != want {
			t.Fatalf("shell env[%q] = %q, want %q (env=%#v)", key, env[key], want, env)
		}
	}
}
