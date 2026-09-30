package websocket

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
	_ "github.com/mattn/go-sqlite3"
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

func TestStartUserShellProcessExecutesPersistedTerminalCommand(t *testing.T) {
	log := testTerminalLogger(t)
	manager := lifecycle.NewManager(nil, bus.NewMemoryEventBus(log), nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	execution := &lifecycle.AgentExecution{
		ID: "exec-auth", TaskID: "task-auth", SessionID: "session-auth",
		TaskEnvironmentID: "env-auth", WorkspacePath: t.TempDir(),
	}
	if err := manager.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetExecutionEnv(context.Background(), execution.ID, map[string]string{"PATH": os.Getenv("PATH")}); err != nil {
		t.Fatal(err)
	}

	rawDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "terminals.db"))
	if err != nil {
		t.Fatal(err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := terminalrepo.NewWithDB(db, db, log)
	if err != nil {
		t.Fatal(err)
	}
	runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)
	terminals := terminalservice.New(repo, terminalservice.NewInteractiveRunnerBackend(runner), log)
	marker := filepath.Join(t.TempDir(), "login-command-ran")
	command := "printf x >> " + shellQuoteForTerminalTest(marker) + "; exit"
	terminal, err := terminals.CreateWithOneShotInitialCommand(context.Background(), execution.TaskID, execution.TaskEnvironmentID, command)
	if err != nil {
		t.Fatal(err)
	}
	label := "Sign in to MCP"
	if err := terminals.Rename(context.Background(), execution.TaskID, terminal.ID, &label); err != nil {
		t.Fatal(err)
	}
	handler := NewTerminalHandler(manager, nil, nil, log)
	handler.SetTerminalService(terminals)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal/environment/env-auth?terminalId="+terminal.ID+"&label=untrusted", nil)
	processID, status, message := handler.startUserShellProcess(c, execution, execution.TaskEnvironmentID, terminal.ID, runner)
	if message != "" || status != 0 || processID == "" {
		t.Fatalf("startUserShellProcess() = (%q, %d, %q)", processID, status, message)
	}
	if err := runner.ResizeUserShell(execution.TaskEnvironmentID, terminal.ID, 80, 24); err != nil {
		t.Fatalf("start initial PTY: %v", err)
	}
	waitForTerminalMarker(t, marker, "x")
	waitForTerminalProcessExit(t, runner, processID)

	// The terminal WebSocket reconnects after the login process exits. It gets
	// an explicit normal close reason rather than a blank shell that would look
	// live to Authenticate and suppress the next login attempt.
	terminalRouter := gin.New()
	terminalRouter.GET("/terminal", func(c *gin.Context) {
		handler.handleUserShellWS(c, execution, execution.TaskEnvironmentID, terminal.ID, runner)
	})
	terminalServer := httptest.NewServer(terminalRouter)
	t.Cleanup(terminalServer.Close)
	wsURL := "ws" + strings.TrimPrefix(terminalServer.URL, "http") + "/terminal?terminalId=" + terminal.ID
	wsConn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("reconnect WebSocket: %v", err)
	}
	_, _, err = wsConn.ReadMessage()
	_ = wsConn.Close()
	var closeErr *gorillaws.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != gorillaws.CloseNormalClosure || closeErr.Text != "initial_command_completed" {
		t.Fatalf("reconnect close = %#v, error %v", closeErr, err)
	}
	if got := readTerminalMarker(t, marker); got != "x" {
		t.Fatalf("automatic reconnect replayed one-shot login: marker = %q, want x", got)
	}

	// A later explicit Authenticate creates a new terminal and may run a fresh
	// one-shot login command.
	nextTerminal, err := terminals.CreateWithOneShotInitialCommand(context.Background(), execution.TaskID, execution.TaskEnvironmentID, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminals.Rename(context.Background(), execution.TaskID, nextTerminal.ID, &label); err != nil {
		t.Fatal(err)
	}
	nextRecorder := httptest.NewRecorder()
	nextContext, _ := gin.CreateTestContext(nextRecorder)
	nextContext.Request = httptest.NewRequest(http.MethodGet, "/terminal/environment/env-auth?terminalId="+nextTerminal.ID, nil)
	nextProcessID, status, message := handler.startUserShellProcess(nextContext, execution, execution.TaskEnvironmentID, nextTerminal.ID, runner)
	if message != "" || status != 0 || nextProcessID == "" {
		t.Fatalf("explicit next startUserShellProcess() = (%q, %d, %q)", nextProcessID, status, message)
	}
	if err := runner.ResizeUserShell(execution.TaskEnvironmentID, nextTerminal.ID, 80, 24); err != nil {
		t.Fatalf("start explicit next PTY: %v", err)
	}
	waitForTerminalMarker(t, marker, "xx")
	waitForTerminalProcessExit(t, runner, nextProcessID)
}

func waitForTerminalMarker(t *testing.T, marker, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := readTerminalMarker(t, marker); got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("terminal command marker = %q, want %q", readTerminalMarker(t, marker), want)
}

func readTerminalMarker(t *testing.T, marker string) string {
	t.Helper()
	data, err := os.ReadFile(marker)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read marker: %v", err)
	}
	return string(data)
}

func waitForTerminalProcessExit(t *testing.T, runner *process.InteractiveRunner, processID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !runner.IsProcessRunning(processID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("terminal process %s did not exit", processID)
}

func shellQuoteForTerminalTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
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

func (s *stubExecutorProfileReader) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if s.session == nil {
		return &taskmodels.Task{ID: id}, nil
	}
	return &taskmodels.Task{ID: s.session.TaskID}, nil
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

func TestStartUserShellProcessFailsClosedOnExecutorProfileSecretFailure(t *testing.T) {
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
		env: &taskmodels.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
		profile: &taskmodels.ExecutorProfile{
			ID:      "prof-1",
			EnvVars: []taskmodels.ProfileEnvVar{{Key: "TOKEN", SecretID: "deleted-secret"}},
		},
	})
	handler := NewTerminalHandler(manager, nil, nil, log)
	runner := process.NewInteractiveRunner(nil, log, 2*1024*1024)

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
	if processID != "" || status != http.StatusServiceUnavailable || errMsg != "executor profile environment unavailable" {
		t.Fatalf("startUserShellProcess() = (%q, %d, %q), want service unavailable without a process", processID, status, errMsg)
	}
	if shells := runner.ListUserShells("env-1"); len(shells) != 0 {
		t.Fatalf("secret failure started user shells: %#v", shells)
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
