package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	PublishedEvents []*bus.Event
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.PublishedEvents = append(m.PublishedEvents, event)
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {}

func (m *MockEventBus) IsConnected() bool {
	return true
}

// MockCredentialsManager implements lifecycle.CredentialsManager for testing
type MockCredentialsManager struct{}

func (m *MockCredentialsManager) GetCredentialValue(ctx context.Context, key string) (string, error) {
	return "", nil
}

// MockProfileResolver implements lifecycle.ProfileResolver for testing
type MockProfileResolver struct{}

func (m *MockProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*lifecycle.AgentProfileInfo, error) {
	return &lifecycle.AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Test Profile",
		AgentID:     "augment-agent",
		AgentName:   "auggie",
		Model:       "claude-sonnet-4-20250514",
	}, nil
}

func newTestRegistry() *registry.Registry {
	log := newTestLogger()
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	return reg
}

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestNewShellHandlers(t *testing.T) {
	log := newTestLogger()

	// NewShellHandlers accepts *lifecycle.Manager, but we can pass nil for basic construction test
	handlers := NewShellHandlers(nil, nil, log)

	if handlers == nil {
		t.Fatal("expected non-nil handlers")
	} else {
		if handlers.lifecycleMgr != nil {
			t.Error("expected nil lifecycleMgr when nil passed")
		}
		if handlers.logger == nil {
			t.Error("expected non-nil logger")
		}
	}
}

func TestRegisterHandlers(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	// Check that shell.status is registered
	if !dispatcher.HasHandler(ws.ActionShellStatus) {
		t.Error("expected shell.status handler to be registered")
	}

	// Check that shell.input is registered
	if !dispatcher.HasHandler(ws.ActionShellInput) {
		t.Error("expected shell.input handler to be registered")
	}
}

func TestWsShellStatus_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	// Create a message with invalid payload
	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionShellStatus,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsShellStatus(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsShellStatus_MissingTaskID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	// Create a message with empty session_id
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{SessionID: ""})

	_, err := handlers.wsShellStatus(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing session_id")
	}
	if err.Error() != "session_id is required" {
		t.Errorf("expected 'session_id is required' error, got: %v", err)
	}
}

func TestWsShellInput_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionShellInput,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsShellInput_MissingTaskID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellInput, ShellInputRequest{SessionID: "", Data: "test"})

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing session_id")
	}
	if err.Error() != "session_id is required" {
		t.Errorf("expected 'session_id is required' error, got: %v", err)
	}
}

// newTestManager creates a lifecycle.Manager for testing
func newTestManager() *lifecycle.Manager {
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBus{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}
	// Pass nil for runtime - tests don't need them
	return lifecycle.NewManager(reg, eventBus, nil, credsMgr, profileResolver, nil, lifecycle.ExecutorFallbackWarn, "", log)
}

func TestWsShellStatus_NoInstanceFound(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{SessionID: "session-1"})

	resp, err := handlers.wsShellStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return a response indicating no agent available
	var payload map[string]interface{}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload["available"] != false {
		t.Errorf("expected available=false, got %v", payload["available"])
	}
	errorStr, ok := payload["error"].(string)
	if !ok || !strings.Contains(errorStr, "no agent running for this session") {
		t.Errorf("expected error to contain 'no agent running for this session', got %v", payload["error"])
	}
}

func TestWsShellStatus_NoClientAvailable(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	// When no instance exists for the task, handler returns "no agent running"
	// Note: Testing the "agent client not available" path requires injecting an
	// instance with nil agentctl client, which requires access to lifecycle.Manager
	// internal maps. This test verifies the handler works correctly with the manager.
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{SessionID: "session-1"})

	resp, err := handlers.wsShellStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Since there's no instance, we expect the "no agent running" error
	var payload map[string]interface{}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload["available"] != false {
		t.Errorf("expected available=false, got %v", payload["available"])
	}
}

func TestWsShellSubscribe_NoInstanceFound(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellSubscribe, ShellSubscribeRequest{SessionID: "session-1"})

	_, err := handlers.wsShellSubscribe(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	expectedPrefix := "no agent running for session session-1"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Errorf("expected prefix '%s', got: %v", expectedPrefix, err)
	}
}

func TestWsShellInput_NoInstanceFound(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellInput, ShellInputRequest{
		SessionID: "session-1",
		Data:      "test input",
	})

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for non-existent session")
	}
	expectedErr := "no agent running for session session-1"
	if err.Error() != expectedErr {
		t.Errorf("expected '%s', got: %v", expectedErr, err)
	}
}

func TestNewShellHandlers_WithManager(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()

	handlers := NewShellHandlers(mgr, nil, log)

	if handlers == nil {
		t.Fatal("expected non-nil handlers")
	} else if handlers.lifecycleMgr != mgr {
		t.Error("expected lifecycleMgr to be set to provided manager")
	}
}

func TestRegisterHandlers_DispatcherReceivesMessages(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	// Test that dispatcher can dispatch to shell.status
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{SessionID: ""})
	_, err := dispatcher.Dispatch(context.Background(), msg)

	// Should get "session_id is required" error since we passed empty session ID
	if err == nil {
		t.Error("expected error from dispatcher")
	}
}

// --- User Shell Handler Tests ---

func TestRegisterHandlers_UserShellActions(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	// Verify user shell actions are registered
	if !dispatcher.HasHandler(ws.ActionUserShellList) {
		t.Error("expected user_shell.list handler to be registered")
	}
	if !dispatcher.HasHandler(ws.ActionUserShellCreate) {
		t.Error("expected user_shell.create handler to be registered")
	}
	if !dispatcher.HasHandler(ws.ActionUserShellStop) {
		t.Error("expected user_shell.stop handler to be registered")
	}
}

func TestWsUserShellList_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionUserShellList,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsUserShellList(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsUserShellList_MissingTaskEnvironmentID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellList, UserShellListRequest{TaskEnvironmentID: ""})

	_, err := handlers.wsUserShellList(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing task_environment_id")
	}
	if err.Error() != "task_environment_id is required" {
		t.Errorf("expected 'task_environment_id is required', got: %v", err)
	}
}

func TestWsUserShellList_NoInteractiveRunner(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellList, UserShellListRequest{TaskEnvironmentID: "env-1"})

	resp, err := handlers.wsUserShellList(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty shells since interactive runner is not set up
	var payload map[string]interface{}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	shells, ok := payload["shells"]
	if !ok {
		t.Error("expected 'shells' in response")
	}
	shellList, ok := shells.([]interface{})
	if !ok {
		t.Errorf("expected shells to be an array, got %T", shells)
	}
	if len(shellList) != 0 {
		t.Errorf("expected empty shells array, got %d items", len(shellList))
	}
}

func TestWsUserShellCreate_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionUserShellCreate,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsUserShellCreate(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsUserShellCreate_MissingTaskEnvironmentID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellCreate, UserShellCreateRequest{TaskEnvironmentID: ""})

	_, err := handlers.wsUserShellCreate(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing task_environment_id")
	}
	if err.Error() != "task_environment_id is required" {
		t.Errorf("expected 'task_environment_id is required', got: %v", err)
	}
}

func TestWsUserShellCreate_NoInteractiveRunner(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellCreate, UserShellCreateRequest{TaskEnvironmentID: "env-1"})

	_, err := handlers.wsUserShellCreate(context.Background(), msg)
	if err == nil {
		t.Error("expected error when interactive runner not available")
	}
	if err.Error() != "interactive runner not available" {
		t.Errorf("expected 'interactive runner not available', got: %v", err)
	}
}

func TestWsUserShellCreate_ScriptID_NoScriptService(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	// Pass nil for script service
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellCreate, UserShellCreateRequest{
		TaskEnvironmentID: "env-1",
		ScriptID:          "script-123",
	})

	// This will fail because interactive runner is nil (checked before script service),
	// so the error is "interactive runner not available"
	_, err := handlers.wsUserShellCreate(context.Background(), msg)
	if err == nil {
		t.Error("expected error")
	}
}

func TestResolveShellScript(t *testing.T) {
	log := newTestLogger()
	h := NewShellHandlers(nil, nil, log)
	ctx := context.Background()

	t.Run("plain shell when no script_id and no command", func(t *testing.T) {
		label, cmd, err := h.resolveShellScript(ctx, &UserShellCreateRequest{TaskEnvironmentID: "env-1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if label != "" || cmd != "" {
			t.Errorf("expected empty label/command, got label=%q command=%q", label, cmd)
		}
	})

	t.Run("inline command with explicit label", func(t *testing.T) {
		label, cmd, err := h.resolveShellScript(ctx, &UserShellCreateRequest{
			TaskEnvironmentID: "env-1",
			Command:           "echo hi",
			Label:             "Dev Server",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if label != "Dev Server" {
			t.Errorf("label = %q, want %q", label, "Dev Server")
		}
		if cmd != "echo hi" {
			t.Errorf("command = %q, want %q", cmd, "echo hi")
		}
	})

	t.Run("inline command defaults label to Script", func(t *testing.T) {
		label, cmd, err := h.resolveShellScript(ctx, &UserShellCreateRequest{
			TaskEnvironmentID: "env-1",
			Command:           "ls",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if label != "Script" {
			t.Errorf("label = %q, want %q", label, "Script")
		}
		if cmd != "ls" {
			t.Errorf("command = %q, want %q", cmd, "ls")
		}
	})

	t.Run("script_id without script service errors", func(t *testing.T) {
		_, _, err := h.resolveShellScript(ctx, &UserShellCreateRequest{
			TaskEnvironmentID: "env-1",
			ScriptID:          "abc",
		})
		if err == nil {
			t.Fatal("expected error when script service is nil")
		}
	})
}

func TestWsUserShellStop_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionUserShellStop,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsUserShellStop(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsUserShellStop_MissingTaskEnvironmentID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellStop, UserShellStopRequest{TaskEnvironmentID: "", TerminalID: "term-1"})

	_, err := handlers.wsUserShellStop(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing task_environment_id")
	}
	if err.Error() != "task_environment_id is required" {
		t.Errorf("expected 'task_environment_id is required', got: %v", err)
	}
}

func TestWsUserShellStop_MissingTerminalID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellStop, UserShellStopRequest{TaskEnvironmentID: "env-1", TerminalID: ""})

	_, err := handlers.wsUserShellStop(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing terminal_id")
	}
	if err.Error() != "terminal_id is required" {
		t.Errorf("expected 'terminal_id is required', got: %v", err)
	}
}

func TestWsUserShellStop_NoInteractiveRunner(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionUserShellStop, UserShellStopRequest{
		TaskEnvironmentID: "env-1",
		TerminalID:        "term-1",
	})

	_, err := handlers.wsUserShellStop(context.Background(), msg)
	if err == nil {
		t.Error("expected error when interactive runner not available")
	}
	if err.Error() != "interactive runner not available" {
		t.Errorf("expected 'interactive runner not available', got: %v", err)
	}
}

// stubShellLifecycle satisfies shellLifecycle for ensureShellExecution
// tests so the recovery branch can be driven without a real
// lifecycle.Manager. Each call records its argument and returns the next
// scripted return value.
type stubShellLifecycle struct {
	getResults   []*lifecycle.AgentExecution
	getErrors    []error
	getCalls     []string
	cleanupErr   error
	cleanupCalls []string
}

func (s *stubShellLifecycle) GetOrEnsureExecution(_ context.Context, sessionID string) (*lifecycle.AgentExecution, error) {
	idx := len(s.getCalls)
	s.getCalls = append(s.getCalls, sessionID)
	if idx >= len(s.getResults) {
		return nil, errors.New("stub exhausted")
	}
	return s.getResults[idx], s.getErrors[idx]
}

func (s *stubShellLifecycle) CleanupStaleExecutionBySessionID(_ context.Context, sessionID string) error {
	s.cleanupCalls = append(s.cleanupCalls, sessionID)
	return s.cleanupErr
}

func TestEnsureShellExecution_HappyPath(t *testing.T) {
	stub := &stubShellLifecycle{
		getResults: []*lifecycle.AgentExecution{{ID: "exec-1", Status: v1.AgentStatusRunning}},
		getErrors:  []error{nil},
	}

	got, err := ensureShellExecution(context.Background(), stub, newTestLogger(), "session-1")
	if err != nil {
		t.Fatalf("happy-path returned error: %v", err)
	}
	if got.ID != "exec-1" {
		t.Errorf("got execution %q, want exec-1", got.ID)
	}
	if len(stub.getCalls) != 1 {
		t.Errorf("expected 1 GetOrEnsureExecution call, got %d", len(stub.getCalls))
	}
	if len(stub.cleanupCalls) != 0 {
		t.Errorf("running execution should not trigger cleanup, got %d calls", len(stub.cleanupCalls))
	}
}

func TestEnsureShellExecution_RecoversStaleExecution(t *testing.T) {
	// First call returns a Failed execution → cleanup → second call returns
	// a healthy one. This is the recovery path that survives a crashed
	// agent: the user retries shell ops, kandev cleans the stale row, and
	// re-launches transparently.
	stub := &stubShellLifecycle{
		getResults: []*lifecycle.AgentExecution{
			{ID: "exec-stale", Status: v1.AgentStatusFailed},
			{ID: "exec-fresh", Status: v1.AgentStatusRunning},
		},
		getErrors: []error{nil, nil},
	}

	got, err := ensureShellExecution(context.Background(), stub, newTestLogger(), "session-1")
	if err != nil {
		t.Fatalf("recovery path returned error: %v", err)
	}
	if got.ID != "exec-fresh" {
		t.Errorf("got %q, want exec-fresh after recovery", got.ID)
	}
	if len(stub.cleanupCalls) != 1 || stub.cleanupCalls[0] != "session-1" {
		t.Errorf("expected exactly one CleanupStaleExecutionBySessionID(session-1), got %v", stub.cleanupCalls)
	}
	if len(stub.getCalls) != 2 {
		t.Errorf("expected 2 GetOrEnsureExecution calls (initial + recovery), got %d", len(stub.getCalls))
	}
}

func TestEnsureShellExecution_InitialErrorWrapped(t *testing.T) {
	stub := &stubShellLifecycle{
		getResults: []*lifecycle.AgentExecution{nil},
		getErrors:  []error{errors.New("manager unavailable")},
	}

	_, err := ensureShellExecution(context.Background(), stub, newTestLogger(), "session-1")
	if err == nil {
		t.Fatal("expected error when initial GetOrEnsureExecution fails")
	}
	if !strings.Contains(err.Error(), "no agent running for session session-1") {
		t.Errorf("error message does not surface session id: %v", err)
	}
}

func TestEnsureShellExecution_CleanupErrorPropagates(t *testing.T) {
	stub := &stubShellLifecycle{
		getResults: []*lifecycle.AgentExecution{{ID: "exec-stale", Status: v1.AgentStatusFailed}},
		getErrors:  []error{nil},
		cleanupErr: errors.New("cleanup denied"),
	}

	_, err := ensureShellExecution(context.Background(), stub, newTestLogger(), "session-1")
	if err == nil {
		t.Fatal("expected cleanup failure to surface")
	}
	if !strings.Contains(err.Error(), "cleanup stale execution") || !strings.Contains(err.Error(), "session-1") {
		t.Errorf("cleanup error not propagated cleanly: %v", err)
	}
	// Second GetOrEnsureExecution must NOT fire when cleanup fails: re-
	// launching on a stuck stale row would loop forever.
	if len(stub.getCalls) != 1 {
		t.Errorf("expected exactly 1 GetOrEnsureExecution call when cleanup fails, got %d", len(stub.getCalls))
	}
}

func TestEnsureShellExecution_RecoveryRelaunchFailureWrapped(t *testing.T) {
	stub := &stubShellLifecycle{
		getResults: []*lifecycle.AgentExecution{
			{ID: "exec-stale", Status: v1.AgentStatusFailed},
			nil,
		},
		getErrors: []error{nil, errors.New("relaunch failed")},
	}

	_, err := ensureShellExecution(context.Background(), stub, newTestLogger(), "session-1")
	if err == nil {
		t.Fatal("expected error when recovery re-launch fails")
	}
	if !strings.Contains(err.Error(), "recover execution") {
		t.Errorf("recovery error not labeled clearly: %v", err)
	}
}
