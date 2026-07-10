package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// overloaded529 is the exact transient 529 Overloaded envelope the
// claude-agent-acp adapter surfaces over ACP as a prompt-time error event.
const overloaded529 = `{"code":-32603,"message":"Internal error: API Error: 529 Overloaded. ` +
	`This is a server-side issue, usually temporary — try again in a moment.","data":{"errorKind":"server_error"}}`

func newTransientTestService(t *testing.T) (*Service, *mockMessageCreator) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	return svc, mc
}

func TestHandleTransientFailure_SchedulesRetryAndEmitsWarning(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	took := svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:       "t1",
		SessionID:    "s1",
		ErrorMessage: overloaded529,
	})
	if !took {
		t.Fatal("handleTransientFailure = false, want true (should own the transient failure)")
	}

	// A retry entry must be armed for the session.
	if _, ok := svc.transientRetries.Load("s1"); !ok {
		t.Fatal("expected a transient retry entry for s1")
	}

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 status message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if msg.metadata["variant"] != "warning" {
		t.Errorf("variant = %v, want warning", msg.metadata["variant"])
	}
	if msg.metadata["retrying"] != true {
		t.Errorf("retrying = %v, want true", msg.metadata["retrying"])
	}
	if msg.metadata["attempt"] != 1 {
		t.Errorf("attempt = %v, want 1", msg.metadata["attempt"])
	}
	if msg.metadata["max_attempts"] != transientMaxAttempts {
		t.Errorf("max_attempts = %v, want %d", msg.metadata["max_attempts"], transientMaxAttempts)
	}
	if msg.metadata["retry_in_seconds"] != 5 {
		t.Errorf("retry_in_seconds = %v, want 5", msg.metadata["retry_in_seconds"])
	}
	if !strings.Contains(strings.ToLower(msg.content), "retrying") {
		t.Errorf("content = %q, want a 'retrying' message", msg.content)
	}
	// Must NOT be the red recovery banner.
	if msg.metadata["recovery_actions"] == true {
		t.Errorf("transient retry must not set recovery_actions=true (that is the red banner)")
	}
	// Must carry a Cancel action.
	actions, ok := msg.metadata["actions"].([]map[string]interface{})
	if !ok || actionByTestID(actions, recoveryCancelRetryButtonTestID) == nil {
		t.Errorf("expected a cancel action with test_id %q, got %v", recoveryCancelRetryButtonTestID, msg.metadata["actions"])
	}
}

func TestTransientFailedExecutionToolUpdateDoesNotCreateMessage(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	svc.handleAgentFailed(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     overloaded529,
	})

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tc1",
			ToolStatus: agentEventComplete,
		},
	})

	if mc.toolUpdateWrites != 0 {
		t.Fatalf("expected stale transient-failure tool update to be dropped, got %d writes", mc.toolUpdateWrites)
	}
}

func TestHandleTransientFailure_NonTransientReturnsFalse(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	took := svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:       "t1",
		SessionID:    "s1",
		ErrorMessage: "agent crashed with exit code 1",
	})
	if took {
		t.Fatal("handleTransientFailure = true for a non-transient error, want false")
	}
	if len(mc.sessionMessages) != 0 {
		t.Errorf("expected no status message for non-transient error, got %d", len(mc.sessionMessages))
	}
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected no retry entry for a non-transient error")
	}
}

func TestHandleTransientFailure_ExhaustedFallsThrough(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	// Pre-seed an entry that has already used the full budget.
	svc.transientRetries.Store("s1", &transientRetryEntry{attempt: transientMaxAttempts, cancel: func() {}})

	took := svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:       "t1",
		SessionID:    "s1",
		ErrorMessage: overloaded529,
	})
	if took {
		t.Fatal("handleTransientFailure = true after budget exhausted, want false (fall through to recovery)")
	}
	if len(mc.sessionMessages) != 0 {
		t.Errorf("expected no new retry status message on exhaustion, got %d", len(mc.sessionMessages))
	}
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected retry entry cleared on exhaustion")
	}
}

func TestCancelTransientRetry_StopsLoopAndShowsRecovery(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	svc.scheduleTransientRetry("t1", "s1", "", 1, 5*time.Second)
	if _, ok := svc.transientRetries.Load("s1"); !ok {
		t.Fatal("expected an armed retry entry before cancel")
	}

	cancelled := svc.CancelTransientRetry(context.Background(), "t1", "s1")
	if !cancelled {
		t.Fatal("CancelTransientRetry = false, want true (a loop was active)")
	}
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected retry entry cleared after cancel")
	}

	// Cancel surfaces the red recovery banner so the user can recover manually.
	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 recovery status message after cancel, got %d", len(mc.sessionMessages))
	}
	if mc.sessionMessages[0].metadata["recovery_actions"] != true {
		t.Errorf("expected recovery_actions=true after cancel, got %v", mc.sessionMessages[0].metadata["recovery_actions"])
	}
}

func TestCancelTransientRetry_NoActiveLoop(t *testing.T) {
	svc, _ := newTransientTestService(t)
	if svc.CancelTransientRetry(context.Background(), "t1", "s1") {
		t.Error("CancelTransientRetry = true with no active loop, want false")
	}
}

func TestResetTransientRetry_ClearsEntry(t *testing.T) {
	svc, _ := newTransientTestService(t)
	svc.scheduleTransientRetry("t1", "s1", "", 1, 5*time.Second)
	svc.resetTransientRetry("s1")
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected entry cleared after resetTransientRetry")
	}
}

func TestResetTransientRetry_DropsCachedPrompt(t *testing.T) {
	svc, _ := newTransientTestService(t)
	// The cached prompt can hold large/sensitive attachment data — it must be
	// released when the retry loop ends, not retained for the backend's life.
	svc.rememberTurnPrompt("s1", "hello", "", false, nil)
	if _, ok := svc.lastTurnPrompt.Load("s1"); !ok {
		t.Fatal("expected a cached prompt")
	}
	svc.resetTransientRetry("s1")
	if _, ok := svc.lastTurnPrompt.Load("s1"); ok {
		t.Error("expected cached prompt cleared after resetTransientRetry")
	}
}

func TestRetryTransientPrompt_NoCachedPromptSurfacesRecovery(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	// Arm a retry entry but never cache a prompt (an uncached launch path).
	svc.scheduleTransientRetry("t1", "s1", "", 1, time.Hour)

	// The timer firing with no cached prompt must NOT leave the loop parked —
	// it clears the entry and surfaces the manual recovery banner.
	svc.retryTransientPrompt(context.Background(), "t1", "s1", "")

	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected retry entry cleared when no prompt is cached")
	}
	hasRecovery := false
	for _, m := range mc.sessionMessages {
		if m.metadata["recovery_actions"] == true {
			hasRecovery = true
		}
	}
	if !hasRecovery {
		t.Error("expected a recovery_actions banner after an uncached retry")
	}
}

func TestRetryTransientPrompt_SynchronousPromptErrorSurfacesRecovery(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr:              errors.New("session rejected prompt synchronously"),
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	t.Cleanup(svc.cancelAllTransientRetries)

	svc.rememberTurnPrompt("s1", "hello", "", false, nil)
	svc.transientRetries.Store("s1", &transientRetryEntry{attempt: 1, cancel: func() {}})

	svc.retryTransientPrompt(context.Background(), "t1", "s1", "exec-1")

	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected retry entry cleared after synchronous PromptTask failure")
	}
	if _, ok := svc.lastTurnPrompt.Load("s1"); ok {
		t.Error("expected cached prompt cleared after synchronous PromptTask failure")
	}
	hasRecovery := false
	for _, m := range mc.sessionMessages {
		if m.metadata["recovery_actions"] == true {
			hasRecovery = true
		}
	}
	if !hasRecovery {
		t.Error("expected recovery banner after synchronous PromptTask failure")
	}
}

func TestTransientRetryEntryClaimPreventsDoubleFire(t *testing.T) {
	entry := &transientRetryEntry{}
	if !entry.claim() {
		t.Fatal("first claim = false, want true")
	}
	if entry.claim() {
		t.Fatal("second claim = true, want false")
	}
}

func TestCancelAllTransientRetries_DropsCachedPrompt(t *testing.T) {
	svc, _ := newTransientTestService(t)
	svc.rememberTurnPrompt("s1", "hello", "", false, nil)
	svc.scheduleTransientRetry("t1", "s1", "", 1, time.Hour)

	svc.cancelAllTransientRetries()

	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Error("expected retry entry cleared after cancelAllTransientRetries")
	}
	if _, ok := svc.lastTurnPrompt.Load("s1"); ok {
		t.Error("expected cached prompt cleared after cancelAllTransientRetries")
	}
}

func TestTransientRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 5 * time.Second}, // clamps up
		{1, 5 * time.Second},
		{2, 15 * time.Second},
		{3, 30 * time.Second},
		{4, 30 * time.Second}, // clamps to last
	}
	for _, tc := range cases {
		if got := transientRetryDelay(tc.attempt); got != tc.want {
			t.Errorf("transientRetryDelay(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestHandleTransientFailure_AttemptCounterIncrements(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", ErrorMessage: overloaded529}

	svc.handleTransientFailure(context.Background(), data)
	svc.handleTransientFailure(context.Background(), data)

	v, ok := svc.transientRetries.Load("s1")
	if !ok {
		t.Fatal("expected retry entry")
	}
	entry, _ := v.(*transientRetryEntry)
	if entry.attempt != 2 {
		t.Errorf("attempt = %d, want 2 after two transient failures", entry.attempt)
	}
}
