package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/client"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// mockExecutionLookup implements ExecutionLookup for testing.
type mockExecutionLookup struct {
	executions map[string]*lifecycle.AgentExecution
	// ensureErr allows tests to inject specific errors from GetOrEnsureExecution
	ensureErr error
}

func (m *mockExecutionLookup) GetExecutionBySessionID(sessionID string) (*lifecycle.AgentExecution, bool) {
	if m.executions == nil {
		return nil, false
	}
	exec, ok := m.executions[sessionID]
	return exec, ok
}

func (m *mockExecutionLookup) GetOrEnsureExecution(_ context.Context, sessionID string) (*lifecycle.AgentExecution, error) {
	// Return injected error if set
	if m.ensureErr != nil {
		return nil, m.ensureErr
	}
	if m.executions == nil {
		return nil, fmt.Errorf("no execution for session %s", sessionID)
	}
	exec, ok := m.executions[sessionID]
	if !ok {
		return nil, fmt.Errorf("no execution for session %s", sessionID)
	}
	return exec, nil
}

// mockSessionReader implements SessionReader for testing.
type mockSessionReader struct {
	baseCommits  map[string]string
	baseBranches map[string]string
}

func (m *mockSessionReader) GetSessionBaseCommit(_ context.Context, sessionID string) string {
	if m.baseCommits == nil {
		return ""
	}
	return m.baseCommits[sessionID]
}

func (m *mockSessionReader) GetSessionBaseBranch(_ context.Context, sessionID string) string {
	if m.baseBranches == nil {
		return ""
	}
	return m.baseBranches[sessionID]
}

func TestNewGitHandlers(t *testing.T) {
	log := newTestLogger()
	lookup := &mockExecutionLookup{}
	reader := &mockSessionReader{}

	h := NewGitHandlers(lookup, reader, log)
	if h == nil {
		t.Fatal("expected non-nil handlers")
	}
	if h.lifecycleMgr != lookup {
		t.Error("expected lifecycleMgr to match")
	}
	if h.sessionReader != reader {
		t.Error("expected sessionReader to match")
	}
}

func TestNewGitHandlers_NilDependencies(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(nil, nil, log)
	if h == nil {
		t.Fatal("expected non-nil handlers")
	}
}

func TestRegisterGitHandlers(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(nil, nil, log)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)

	actions := []string{
		ws.ActionWorktreePull,
		ws.ActionWorktreePush,
		ws.ActionWorktreeRebase,
		ws.ActionWorktreeMerge,
		ws.ActionWorktreeAbort,
		ws.ActionWorktreeCommit,
		ws.ActionWorktreeStage,
		ws.ActionWorktreeUnstage,
		ws.ActionWorktreeDiscard,
		ws.ActionWorktreeCreatePR,
		ws.ActionWorktreeRevertCommit,
		ws.ActionWorktreeRenameBranch,
		ws.ActionWorktreeReset,
		ws.ActionSessionCommitDiff,
		ws.ActionSessionGitCommits,
		ws.ActionSessionCumulativeDiff,
	}
	for _, action := range actions {
		if !d.HasHandler(action) {
			t.Errorf("expected handler registered for %s", action)
		}
	}
}

func TestNotifyGitOperationFailed_NilResult(t *testing.T) {
	log := newTestLogger()
	called := false
	h := NewGitHandlers(&mockExecutionLookup{}, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, _, _ string) {
		called = true
	})

	// Should not panic or call callback
	h.notifyGitOperationFailed("session-1", "commit", nil)
	if called {
		t.Error("callback should not be called for nil result")
	}
}

func TestNotifyGitOperationFailed_SuccessResult(t *testing.T) {
	log := newTestLogger()
	called := false
	h := NewGitHandlers(&mockExecutionLookup{}, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, _, _ string) {
		called = true
	})

	h.notifyGitOperationFailed("session-1", "commit", &client.GitOperationResult{
		Success: true,
	})
	if called {
		t.Error("callback should not be called for successful result")
	}
}

func TestNotifyGitOperationFailed_NilCallback(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(&mockExecutionLookup{}, nil, log)
	// No callback set — should not panic
	h.notifyGitOperationFailed("session-1", "commit", &client.GitOperationResult{
		Success: false,
		Error:   "pre-commit hook failed",
	})
}

func TestNotifyGitOperationFailed_NoExecution(t *testing.T) {
	log := newTestLogger()
	called := false
	h := NewGitHandlers(&mockExecutionLookup{}, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, _, _ string) {
		called = true
	})

	h.notifyGitOperationFailed("unknown-session", "commit", &client.GitOperationResult{
		Success: false,
		Error:   "failed",
	})

	// Give the goroutine a moment (it shouldn't fire)
	time.Sleep(50 * time.Millisecond)
	if called {
		t.Error("callback should not be called when no execution found")
	}
}

func TestNotifyGitOperationFailed_EmptyTaskID(t *testing.T) {
	log := newTestLogger()
	called := false
	lookup := &mockExecutionLookup{
		executions: map[string]*lifecycle.AgentExecution{
			"session-1": {ID: "exec-1", SessionID: "session-1", TaskID: ""},
		},
	}
	h := NewGitHandlers(lookup, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, _, _ string) {
		called = true
	})

	h.notifyGitOperationFailed("session-1", "push", &client.GitOperationResult{
		Success: false,
		Error:   "rejected",
	})

	time.Sleep(50 * time.Millisecond)
	if called {
		t.Error("callback should not be called when task ID is empty")
	}
}

func TestNotifyGitOperationFailed_UsesErrorField(t *testing.T) {
	log := newTestLogger()
	var mu sync.Mutex
	var gotOperation, gotErrorOutput string
	lookup := &mockExecutionLookup{
		executions: map[string]*lifecycle.AgentExecution{
			"session-1": {ID: "exec-1", SessionID: "session-1", TaskID: "task-1"},
		},
	}
	h := NewGitHandlers(lookup, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, operation, errorOutput string) {
		mu.Lock()
		defer mu.Unlock()
		gotOperation = operation
		gotErrorOutput = errorOutput
	})

	h.notifyGitOperationFailed("session-1", "commit", &client.GitOperationResult{
		Success: false,
		Error:   "pre-commit hook failed",
		Output:  "some output",
	})

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gotOperation != "commit" {
		t.Errorf("expected operation 'commit', got %q", gotOperation)
	}
	if gotErrorOutput != "pre-commit hook failed" {
		t.Errorf("expected error output 'pre-commit hook failed', got %q", gotErrorOutput)
	}
}

func TestNotifyGitOperationFailed_FallsBackToOutput(t *testing.T) {
	log := newTestLogger()
	var mu sync.Mutex
	var gotErrorOutput string
	lookup := &mockExecutionLookup{
		executions: map[string]*lifecycle.AgentExecution{
			"session-1": {ID: "exec-1", SessionID: "session-1", TaskID: "task-1"},
		},
	}
	h := NewGitHandlers(lookup, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, _, _, _, errorOutput string) {
		mu.Lock()
		defer mu.Unlock()
		gotErrorOutput = errorOutput
	})

	h.notifyGitOperationFailed("session-1", "push", &client.GitOperationResult{
		Success: false,
		Error:   "",
		Output:  "fatal: rejected",
	})

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gotErrorOutput != "fatal: rejected" {
		t.Errorf("expected fallback to Output field, got %q", gotErrorOutput)
	}
}

func TestNotifyGitOperationFailed_PassesSessionAndTaskID(t *testing.T) {
	log := newTestLogger()
	var mu sync.Mutex
	var gotSessionID, gotTaskID string
	lookup := &mockExecutionLookup{
		executions: map[string]*lifecycle.AgentExecution{
			"sess-42": {ID: "exec-1", SessionID: "sess-42", TaskID: "task-99"},
		},
	}
	h := NewGitHandlers(lookup, nil, log)
	h.SetOnGitOperationFailed(func(_ context.Context, sessionID, taskID, _, _ string) {
		mu.Lock()
		defer mu.Unlock()
		gotSessionID = sessionID
		gotTaskID = taskID
	})

	h.notifyGitOperationFailed("sess-42", "rebase", &client.GitOperationResult{
		Success: false,
		Error:   "conflict",
	})

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gotSessionID != "sess-42" {
		t.Errorf("expected sessionID 'sess-42', got %q", gotSessionID)
	}
	if gotTaskID != "task-99" {
		t.Errorf("expected taskID 'task-99', got %q", gotTaskID)
	}
}

func TestWsPull_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(nil, nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionWorktreePull,
		Payload: json.RawMessage(`{invalid`),
	}

	_, err := h.wsPull(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsPull_MissingSessionID(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionWorktreePull, GitPullRequest{SessionID: ""})

	_, err := h.wsPull(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing session_id")
	}
}

func TestWsCommit_MissingMessage(t *testing.T) {
	log := newTestLogger()
	h := NewGitHandlers(nil, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionWorktreeCommit, GitCommitRequest{
		SessionID: "session-1",
		Message:   "",
	})

	_, err := h.wsCommit(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing message")
	}
}

func TestWsPush_NoExecution(t *testing.T) {
	log := newTestLogger()
	lookup := &mockExecutionLookup{}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionWorktreePush, GitPushRequest{SessionID: "session-1"})

	_, err := h.wsPush(context.Background(), msg)
	if err == nil {
		t.Error("expected error when no execution found")
	}
}

func TestWsGitCommits_NotReady(t *testing.T) {
	log := newTestLogger()
	// Use "no agent running for session" error which matches isSessionNotReadyError
	lookup := &mockExecutionLookup{
		ensureErr: fmt.Errorf("no agent running for session session-1"),
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1"})

	resp, err := h.wsGitCommits(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected graceful response, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if ready, ok := payload["ready"].(bool); !ok || ready {
		t.Errorf("expected ready=false, got %v", payload["ready"])
	}
	commits, ok := payload["commits"].([]any)
	if !ok {
		t.Fatalf("expected commits array, got %T", payload["commits"])
	}
	if len(commits) != 0 {
		t.Errorf("expected empty commits, got %d", len(commits))
	}
}

func TestWsCumulativeDiff_NotReady(t *testing.T) {
	log := newTestLogger()
	lookup := &mockExecutionLookup{} // no executions → "no agent running" error
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session-1"})

	resp, err := h.wsCumulativeDiff(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected graceful response, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if ready, ok := payload["ready"].(bool); !ok || ready {
		t.Errorf("expected ready=false, got %v", payload["ready"])
	}
	if payload["cumulative_diff"] != nil {
		t.Errorf("expected cumulative_diff=nil, got %v", payload["cumulative_diff"])
	}
}

func TestIsSessionNotReadyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"no agent running", fmt.Errorf("no agent running for session abc"), true},
		{"agent client not available", fmt.Errorf("agent client not available for session abc"), true},
		{"unrelated error", fmt.Errorf("some other error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSessionNotReadyError(tt.err); got != tt.want {
				t.Errorf("isSessionNotReadyError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWsGitCommits_WorkspaceNotReadyError(t *testing.T) {
	// Test that ErrSessionWorkspaceNotReady returns ready:false gracefully
	log := newTestLogger()
	lookup := &mockExecutionLookup{
		ensureErr: lifecycle.ErrSessionWorkspaceNotReady,
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1"})

	resp, err := h.wsGitCommits(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected graceful response for workspace not ready, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if ready, ok := payload["ready"].(bool); !ok || ready {
		t.Errorf("expected ready=false, got %v", payload["ready"])
	}
}

func TestWsGitCommits_UnexpectedError(t *testing.T) {
	// Test that unexpected errors (not workspace-not-ready) are returned as errors
	log := newTestLogger()
	lookup := &mockExecutionLookup{
		ensureErr: errors.New("database connection failed"),
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1"})

	_, err := h.wsGitCommits(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for unexpected failure")
	}
	if !errors.Is(err, lookup.ensureErr) && err.Error() != "failed to get execution for session session-1: database connection failed" {
		t.Errorf("expected wrapped database error, got: %v", err)
	}
}

func TestWsGitCommits_WrappedWorkspaceNotReadyError(t *testing.T) {
	// Test that wrapped ErrSessionWorkspaceNotReady is still detected
	log := newTestLogger()
	lookup := &mockExecutionLookup{
		ensureErr: fmt.Errorf("some context: %w", lifecycle.ErrSessionWorkspaceNotReady),
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1"})

	resp, err := h.wsGitCommits(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected graceful response for wrapped workspace not ready, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if ready, ok := payload["ready"].(bool); !ok || ready {
		t.Errorf("expected ready=false, got %v", payload["ready"])
	}
}
