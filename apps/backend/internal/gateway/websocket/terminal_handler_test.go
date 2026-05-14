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
	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestCheckWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		// No origin — allow (non-browser client)
		{"no origin", "", "example.com", true},

		// Localhost variants
		{"http://localhost", "http://localhost", "localhost", true},
		{"http://localhost:3000", "http://localhost:3000", "localhost:8080", true},
		{"https://localhost", "https://localhost", "localhost", true},
		{"http://127.0.0.1", "http://127.0.0.1", "127.0.0.1", true},
		{"http://127.0.0.1:3000", "http://127.0.0.1:3000", "127.0.0.1:8080", true},
		{"https://127.0.0.1", "https://127.0.0.1", "127.0.0.1", true},

		// Same-origin (origin host matches request host)
		{"same origin", "https://example.com", "example.com", true},
		{"same origin with port", "https://example.com:443", "example.com:8080", true},

		// Cross-origin — reject
		{"cross origin", "https://evil.com", "example.com", false},
		{"cross origin similar", "https://notexample.com", "example.com", false},

		// Malformed origin
		{"malformed origin", "not-a-url", "example.com", false},

		// IPv6 host
		{"ipv6 cross origin", "http://[::1]:3000", "example.com:8080", false},

		// Empty host — no match possible
		{"empty host rejects", "https://example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header: http.Header{},
				Host:   tt.host,
				URL:    &url.URL{Host: tt.host},
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			got := checkWebSocketOrigin(r)
			if got != tt.want {
				t.Errorf("checkWebSocketOrigin(origin=%q, host=%q) = %v, want %v",
					tt.origin, tt.host, got, tt.want)
			}
		})
	}
}

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
