package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/scripts"
)

// stubScriptService returns a fixed repository script (or error) for any ID.
type stubScriptService struct {
	script *scripts.RepositoryScript
	err    error
	calls  int
}

func (s *stubScriptService) GetRepositoryScript(context.Context, string) (*scripts.RepositoryScript, error) {
	s.calls++
	return s.script, s.err
}

type stubUserService struct {
	shell string
	err   error
}

func (s stubUserService) PreferredShell(context.Context) (string, error) {
	return s.shell, s.err
}

// terminalContext builds a gin context for a terminal route with the wildcard
// :target param the real route uses.
func terminalContext(t *testing.T, target, query string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal"+target+query, nil)
	c.Params = gin.Params{{Key: "target", Value: target}}
	return c, recorder
}

func TestHandleTerminalWSRejectsRoutesWithoutATarget(t *testing.T) {
	const wantError = "terminal route must be /environment/:environmentId for shell terminals " +
		"or /session/:sessionId?mode=agent for agent terminals"

	for _, target := range []string{"/", "/legacy-session-id", "/session/", "/environment/"} {
		t.Run(target, func(t *testing.T) {
			log, _ := observedTerminalLogger(t)
			handler, _ := newBridgeTestHandler(t, log)
			c, recorder := terminalContext(t, target, "")

			handler.HandleTerminalWS(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if got := decodeErrorBody(t, recorder.Body.Bytes()); got != wantError {
				t.Fatalf("error = %q, want %q", got, wantError)
			}
		})
	}
}

func TestHandleTerminalWSValidatesEnvironmentRouteQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantError string
	}{
		{
			name:      "agent mode is not a shell",
			query:     "?mode=agent&terminalId=t-1",
			wantError: "environment terminal route only supports shell mode",
		},
		{
			name:      "missing terminal id",
			query:     "?mode=shell",
			wantError: "terminalId required for shell mode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, _ := observedTerminalLogger(t)
			handler, _ := newBridgeTestHandler(t, log)
			c, recorder := terminalContext(t, "/environment/env-1", tt.query)

			handler.HandleTerminalWS(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if got := decodeErrorBody(t, recorder.Body.Bytes()); got != tt.wantError {
				t.Fatalf("error = %q, want %q", got, tt.wantError)
			}
		})
	}
}

func TestHandleTerminalWSSessionRouteReportsMissingInteractiveRunner(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	c, recorder := terminalContext(t, "/session/session-1", "?mode=agent")

	handler.HandleTerminalWS(c)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "interactive runner not available" {
		t.Fatalf("error = %q", got)
	}
}

func TestHandleAgentTerminalRouteRequiresSessionID(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	c, recorder := terminalContext(t, "/session/", "?mode=agent")

	handler.handleAgentTerminalRoute(c, "")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "sessionId is required" {
		t.Fatalf("error = %q", got)
	}
}

func TestHandleTerminalWSEnvironmentRouteReportsUnreadyExecution(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	c, recorder := terminalContext(t, "/environment/env-unknown", "?terminalId=t-1")

	handler.HandleTerminalWS(c)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got == "" {
		t.Fatal("expected the lifecycle error to be reported to the caller")
	}
}

// The passthrough terminal reaches the lifecycle manager directly, so a denied
// session must be refused before any WebSocket upgrade happens.
func TestHandleAgentPassthroughWSRefusesDeniedSession(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	manager.SetSessionAccessChecker(func(context.Context, string) error {
		return errors.New("session belongs to another user")
	})

	c, recorder := terminalContext(t, "/session/session-denied", "?mode=agent")
	handler.handleAgentPassthroughWS(c, "session-denied", runner)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "session belongs to another user" {
		t.Fatalf("error = %q", got)
	}
	if recorder.Header().Get("Upgrade") != "" {
		t.Fatal("a denied session must not be upgraded to a WebSocket")
	}
}

func TestResolveShellLabelPrefersScriptOverLabel(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	scriptService := &stubScriptService{
		script: &scripts.RepositoryScript{Name: "Dev server", Command: "npm run dev"},
	}
	handler, _ := newBridgeTestHandler(t, log)
	handler.scriptService = scriptService

	c, _ := terminalContext(t, "/environment/env-1", "?scriptId=script-1&label=ignored")
	label, command, httpErr := handler.resolveShellLabel(c)

	if httpErr != "" {
		t.Fatalf("resolveShellLabel() httpErr = %q, want none", httpErr)
	}
	if label != "Dev server" || command != "npm run dev" {
		t.Fatalf("resolveShellLabel() = (%q, %q), want (%q, %q)",
			label, command, "Dev server", "npm run dev")
	}
	if scriptService.calls != 1 {
		t.Fatalf("script lookups = %d, want 1", scriptService.calls)
	}
}

func TestResolveShellLabelReportsInvalidScript(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.scriptService = &stubScriptService{err: errors.New("no such script")}

	c, _ := terminalContext(t, "/environment/env-1", "?scriptId=missing")
	label, command, httpErr := handler.resolveShellLabel(c)

	if httpErr != "invalid script ID" {
		t.Fatalf("resolveShellLabel() httpErr = %q, want %q", httpErr, "invalid script ID")
	}
	if label != "" || command != "" {
		t.Fatalf("resolveShellLabel() = (%q, %q), want empty values on failure", label, command)
	}
}

func TestResolveShellLabelFallsBackToLabelQuery(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	c, _ := terminalContext(t, "/environment/env-1", "?label=Logs")
	if label, command, httpErr := handler.resolveShellLabel(c); label != "Logs" || command != "" || httpErr != "" {
		t.Fatalf("resolveShellLabel() = (%q, %q, %q), want (\"Logs\", \"\", \"\")", label, command, httpErr)
	}

	bare, _ := terminalContext(t, "/environment/env-1", "")
	if label, command, httpErr := handler.resolveShellLabel(bare); label != "" || command != "" || httpErr != "" {
		t.Fatalf("resolveShellLabel() = (%q, %q, %q), want all empty", label, command, httpErr)
	}
}

func TestResolvePreferredShellFallsBackToDefault(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	ctx := context.Background()

	if got := handler.resolvePreferredShell(ctx); got != "" {
		t.Fatalf("resolvePreferredShell() with no user service = %q, want empty", got)
	}

	handler.userService = stubUserService{err: errors.New("settings unavailable")}
	if got := handler.resolvePreferredShell(ctx); got != "" {
		t.Fatalf("resolvePreferredShell() on error = %q, want empty", got)
	}

	handler.userService = stubUserService{shell: "/bin/zsh"}
	if got := handler.resolvePreferredShell(ctx); got != "/bin/zsh" {
		t.Fatalf("resolvePreferredShell() = %q, want %q", got, "/bin/zsh")
	}
}

func TestResolveWorkingDirRequiresAMaterializedWorkspace(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	if _, err := handler.resolveWorkingDir(nil); err == nil {
		t.Fatal("resolveWorkingDir(nil) error = nil, want a failure")
	}

	_, err := handler.resolveWorkingDir(&lifecycle.AgentExecution{TaskEnvironmentID: "env-1"})
	if !errors.Is(err, lifecycle.ErrSessionWorkspaceNotReady) {
		t.Fatalf("resolveWorkingDir() error = %v, want ErrSessionWorkspaceNotReady", err)
	}

	dir := t.TempDir()
	got, err := handler.resolveWorkingDir(&lifecycle.AgentExecution{WorkspacePath: dir})
	if err != nil {
		t.Fatalf("resolveWorkingDir() error = %v", err)
	}
	if got != dir {
		t.Fatalf("resolveWorkingDir() = %q, want %q", got, dir)
	}
}

func TestHandleUserShellWSReportsWorkspaceNotReady(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	c, recorder := terminalContext(t, "/environment/env-1", "?terminalId=t-1")
	execution := &lifecycle.AgentExecution{ID: "exec-1", SessionID: "session-1", TaskEnvironmentID: "env-1"}
	handler.handleUserShellWS(c, execution, "env-1", "t-1", runner)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got == "" {
		t.Fatal("expected the workspace-not-ready reason to be reported")
	}
	if shells := runner.ListUserShells("env-1"); len(shells) != 0 {
		t.Fatalf("user shells = %v, want none when the workspace is not ready", shells)
	}
}

func TestHandleUserShellWSRejectsInvalidScriptBeforeStartingAShell(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.scriptService = &stubScriptService{err: errors.New("no such script")}
	runner := newBridgeTestRunner(t, log)

	c, recorder := terminalContext(t, "/environment/env-1", "?terminalId=t-1&scriptId=missing")
	execution := &lifecycle.AgentExecution{
		ID:                "exec-1",
		SessionID:         "session-1",
		TaskEnvironmentID: "env-1",
		WorkspacePath:     t.TempDir(),
	}
	handler.handleUserShellWS(c, execution, "env-1", "t-1", runner)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "invalid script ID" {
		t.Fatalf("error = %q", got)
	}
	if shells := runner.ListUserShells("env-1"); len(shells) != 0 {
		t.Fatalf("user shells = %v, want none for an invalid script", shells)
	}
}
