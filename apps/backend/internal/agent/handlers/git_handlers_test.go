package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// mockExecutionLookup implements ExecutionLookup for testing.
type mockExecutionLookup struct {
	executions map[string]*lifecycle.AgentExecution
	// ensureErr allows tests to inject specific errors from GetOrEnsureExecution
	ensureErr error
}

// blockingExecutionLookup is a counted, blocking ExecutionLookup used by the
// singleflight dedup tests. Each GetOrEnsureExecution call bumps a counter,
// signals on `entered` (non-blocking), then blocks on `release` so the test
// can fire N concurrent requests and assert exactly one ever runs the inner
// work. Returns `err` once released — set to ErrSessionWorkspaceNotReady so
// the handler returns the graceful not-ready envelope rather than failing.
type blockingExecutionLookup struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
	err     error
}

func (b *blockingExecutionLookup) GetExecutionBySessionID(_ string) (*lifecycle.AgentExecution, bool) {
	return nil, false
}

func (b *blockingExecutionLookup) GetOrEnsureExecution(ctx context.Context, _ string) (*lifecycle.AgentExecution, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case b.entered <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return nil, b.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *blockingExecutionLookup) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
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

func TestIsGitHubPRURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "github", url: "https://github.com/acme/widgets/pull/42", want: true},
		{name: "github enterprise", url: "https://github.acme.corp/acme/widgets/pull/42", want: true},
		{name: "azure repos", url: "https://dev.azure.com/acme/project/_git/widgets/pullrequest/42", want: false},
		{name: "gitlab", url: "https://gitlab.com/acme/widgets/-/merge_requests/42", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitHubPRURL(tt.url); got != tt.want {
				t.Fatalf("isGitHubPRURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
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
	// Use "no agent running for session" error which matches isSessionNotReadyError
	lookup := &mockExecutionLookup{
		ensureErr: fmt.Errorf("no agent running for session session-1"),
	}
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

func TestWsCumulativeDiff_WorkspaceNotReady(t *testing.T) {
	log := newTestLogger()
	lookup := &mockExecutionLookup{
		ensureErr: lifecycle.ErrSessionWorkspaceNotReady,
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session-1"})

	resp, err := h.wsCumulativeDiff(context.Background(), msg)
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
	if payload["cumulative_diff"] != nil {
		t.Errorf("expected cumulative_diff=nil, got %v", payload["cumulative_diff"])
	}
}

func TestWsCumulativeDiff_NilAgentClient(t *testing.T) {
	log := newTestLogger()
	// Execution exists but has no agentctl client yet
	lookup := &mockExecutionLookup{
		executions: map[string]*lifecycle.AgentExecution{
			"session-1": {ID: "exec-1", SessionID: "session-1"},
		},
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session-1"})

	resp, err := h.wsCumulativeDiff(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected graceful response for nil agent client, got error: %v", err)
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

func TestWsCumulativeDiff_UnexpectedError(t *testing.T) {
	log := newTestLogger()
	lookup := &mockExecutionLookup{
		ensureErr: errors.New("database connection failed"),
	}
	h := NewGitHandlers(lookup, nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session-1"})

	_, err := h.wsCumulativeDiff(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for unexpected failure")
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

// TestWsGitCommits_SingleflightDedupesConcurrentRequests verifies that N
// concurrent identical (sessionID, limit) requests collapse onto a single
// computeGitCommits invocation. Without dedup, a frontend re-render storm
// fans out into N agentctl HTTP calls under the per-process throttle.
//
// Runs inside synctest so the assertion fires only after all N waiters
// have called DoChan and the singleflight body has parked — eliminating
// the wall-clock window where a late waiter could miss the dedup and
// trigger a second body invocation.
func TestWsGitCommits_SingleflightDedupesConcurrentRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestLogger()
		lookup := &blockingExecutionLookup{
			entered: make(chan struct{}, 1),
			release: make(chan struct{}),
			err:     lifecycle.ErrSessionWorkspaceNotReady,
		}
		h := NewGitHandlers(lookup, nil, log)

		const N = 8
		type result struct {
			resp *ws.Message
			err  error
		}
		results := make(chan result, N)
		for i := 0; i < N; i++ {
			go func() {
				msg, _ := ws.NewRequest("test", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1", Limit: 50})
				r, err := h.wsGitCommits(context.Background(), msg)
				results <- result{r, err}
			}()
		}

		// Wait until every waiter has parked on its singleflight ch and
		// the body has parked on release. After this, callCount must be
		// exactly 1: any value > 1 means a waiter raced past the dedup.
		synctest.Wait()
		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call before release, got %d — singleflight dedup broken", got)
		}
		close(lookup.release)
		synctest.Wait()

		for i := 0; i < N; i++ {
			r := <-results
			if r.err != nil {
				t.Fatalf("waiter %d got error: %v", i, r.err)
			}
			if r.resp == nil {
				t.Fatalf("waiter %d got nil response", i)
			}
		}

		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call, got %d — singleflight dedup broken", got)
		}
	})
}

// TestWsGitCommits_SingleflightWaiterCancellationDoesNotPoisonOthers verifies
// that cancelling one waiter's request ctx does NOT cancel the shared
// computation or affect other waiters. The shared work runs on a detached
// 60s ctx; individual waiters select on their own ctx via DoChan.
//
// Runs inside synctest so the cancel happens only after every waiter has
// joined the same singleflight call. Without that guarantee, a late waiter
// could race past dedup on slow CI and spawn a second body invocation.
func TestWsGitCommits_SingleflightWaiterCancellationDoesNotPoisonOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestLogger()
		lookup := &blockingExecutionLookup{
			entered: make(chan struct{}, 1),
			release: make(chan struct{}),
			err:     lifecycle.ErrSessionWorkspaceNotReady,
		}
		h := NewGitHandlers(lookup, nil, log)

		const N = 4
		type result struct {
			resp *ws.Message
			err  error
		}
		results := make([]chan result, N)
		ctxs := make([]context.Context, N)
		cancels := make([]context.CancelFunc, N)
		for i := 0; i < N; i++ {
			results[i] = make(chan result, 1)
			ctxs[i], cancels[i] = context.WithCancel(context.Background())
		}
		defer func() {
			for _, c := range cancels {
				c()
			}
		}()

		for i := 0; i < N; i++ {
			go func(idx int) {
				msg, _ := ws.NewRequest("test", ws.ActionSessionGitCommits, GitCommitsRequest{SessionID: "session-1", Limit: 50})
				r, err := h.wsGitCommits(ctxs[idx], msg)
				results[idx] <- result{r, err}
			}(i)
		}

		// Block until every waiter has parked on its singleflight ch and
		// the shared body has parked on release. After this returns, all
		// N waiters share the same in-flight call.
		synctest.Wait()
		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call before cancellation, got %d", got)
		}

		// Cancel waiter 0 while the shared work is still in flight.
		cancels[0]()
		synctest.Wait()
		select {
		case r := <-results[0]:
			if !errors.Is(r.err, context.Canceled) {
				t.Fatalf("waiter 0 expected context.Canceled, got: %v", r.err)
			}
		default:
			t.Fatalf("waiter 0 never observed its ctx cancellation")
		}

		// Release the shared computation — the remaining N-1 waiters must
		// still receive a non-error response.
		close(lookup.release)
		synctest.Wait()
		for i := 1; i < N; i++ {
			select {
			case r := <-results[i]:
				if r.err != nil {
					t.Fatalf("waiter %d got unexpected error after peer cancelled: %v", i, r.err)
				}
				if r.resp == nil {
					t.Fatalf("waiter %d got nil response", i)
				}
			default:
				t.Fatalf("waiter %d never received result", i)
			}
		}

		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call despite cancellation, got %d", got)
		}
	})
}

// TestWsCumulativeDiff_SingleflightDedupesConcurrentRequests mirrors the
// commits test for the cumulative-diff handler, which uses the same DoChan
// + detached-ctx pattern on diffGroup.
func TestWsCumulativeDiff_SingleflightDedupesConcurrentRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestLogger()
		lookup := &blockingExecutionLookup{
			entered: make(chan struct{}, 1),
			release: make(chan struct{}),
			err:     lifecycle.ErrSessionWorkspaceNotReady,
		}
		h := NewGitHandlers(lookup, nil, log)

		const N = 6
		type result struct {
			resp *ws.Message
			err  error
		}
		results := make(chan result, N)
		for i := 0; i < N; i++ {
			go func() {
				msg, _ := ws.NewRequest("test", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session-1"})
				r, err := h.wsCumulativeDiff(context.Background(), msg)
				results <- result{r, err}
			}()
		}

		synctest.Wait()
		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call before release, got %d — singleflight dedup broken on diffGroup", got)
		}
		close(lookup.release)
		synctest.Wait()

		for i := 0; i < N; i++ {
			r := <-results
			if r.err != nil {
				t.Fatalf("waiter %d got error: %v", i, r.err)
			}
			if r.resp == nil {
				t.Fatalf("waiter %d got nil response", i)
			}
		}

		if got := lookup.callCount(); got != 1 {
			t.Fatalf("expected exactly 1 compute call, got %d — singleflight dedup broken on diffGroup", got)
		}
	})
}
