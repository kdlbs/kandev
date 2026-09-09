package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fastPathTaskReadRetryRepo struct {
	*sqliterepo.Repository
	failNext atomic.Bool
}

func (r *fastPathTaskReadRetryRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	if r.failNext.CompareAndSwap(true, false) {
		return nil, errors.New("transient task read failure")
	}
	return r.Repository.GetTask(ctx, taskID)
}

// TestQueueUserPrompt_T2FastPathDrainsPromptableSession pins the
// T2 contract: a user message admitted via QueueUserPrompt triggers
// a synchronous fast-path drain when the session is ready for input,
// no in-flight cancellation/queue/steer holds the guard, and the task
// is not in WIP-wait. The drain uses the existing public helper
// drainQueuedMessageForPromptableSession.
//
// The unit test only verifies the T2 call site is reached (drain's
// ReserveQueued removes the head from the queue, so count drops to
// 0). The downstream dispatch (promptTask) requires a working mock
// agent; full e2e is covered by integration tests.
func TestQueueUserPrompt_T2FastPathDrainsPromptableSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("pre-condition: queue count = %d, want 0", got)
	}
	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "hello", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// drainQueuedMessageForPromptableSession reserves the head before
	// dispatching. The reservation removes the entry from the queue,
	// so the count drops to 0 once T2's fast-path drain is reached.
	// The downstream dispatch may fail (mock agent can't resume) but
	// the count==0 invariant pins the T2 call site.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("post-enqueue queue count = %d, want 0 (T2 fast-path did not drain)", got)
	}
}

// TestQueueUserPrompt_T2SkipsFastPathOnPendingClarification pins the
// live-clarification contract: a session with an active clarification
// request must NOT fast-path drain on enqueue. The drain is deferred to
// the clarification outcome or the next turn boundary.
func TestQueueUserPrompt_T2SkipsFastPathOnPendingClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "during-clarification", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// T2 must NOT drain — the user has not yet answered the
	// clarification. The queue grows; the next turn-end will drain.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("post-enqueue queue count = %d, want 1 (T2 must defer to turn-end on pending clarification)", got)
	}
}

func TestQueueUserPrompt_T2DrainsAfterClarificationDetachedWithEmptyQueue(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	message, err := repo.GetMessage(ctx, "clarification-s1")
	if err != nil {
		t.Fatalf("load clarification: %v", err)
	}
	message.Metadata["agent_disconnected"] = true
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("mark clarification detached: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "after-detach", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("detached-only clarification stranded successor queue: count=%d, want 0", got)
	}
}

func TestQueueUserPrompt_T2RetriesTaskAdmissionReadAfterPromotion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	retryRepo := &fastPathTaskReadRetryRepo{Repository: repo}
	retryRepo.failNext.Store(true)
	svc.repo = retryRepo

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "after-promotion", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("transient task read stranded queue entry: count=%d, want 0", got)
	}
}

// TestQueueUserPrompt_T2SkipsFastPathWhenInFlight pins the in-flight
// guard contract: a queue dispatch already in flight must not be
// raced by the fast-path drain. The drain's internal guard check
// bails when isQueuedDispatchInFlight is true.
func TestQueueUserPrompt_T2SkipsFastPathWhenInFlight(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Simulate an in-flight dispatch: the gate is set by the dispatcher's
	// own markQueuedDispatchInFlight. Set the same internal flag the
	// fast-path checks.
	svc.markQueuedDispatchInFlightWithSourceLocked("s1", "in-flight-source-id", nil)

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "during-in-flight", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// T2 must NOT drain — the drain would race the in-flight dispatch.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("post-enqueue queue count = %d, want 1 (T2 must not race in-flight dispatch)", got)
	}
}

// TestQueueUserPrompt_T2SkipsFastPathOnWIPWait pins the WIP admission
// contract: a task that has not been WIP-admitted and is queued for
// admission (QueuedForStepID != "") must NOT fast-path drain. The
// next admission promotion runs the drain via the existing reconcile
// path; promoting here would bypass WIP gating.
func TestQueueUserPrompt_T2SkipsFastPathOnWIPWait(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// A task in WIP-wait: WIPAdmitted=false, QueuedForStepID="step1".
	task := &models.Task{
		ID:              "t-wip",
		WorkflowID:      "wf1",
		Title:           "WIP-wait task",
		State:           v1.TaskStateInProgress,
		QueuedForStepID: "step1",
		WIPAdmitted:     false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Session in a state that lets the test focus on the WIP guard.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s-wip",
		TaskID:    "t-wip",
		State:     models.TaskSessionStateWaitingForInput,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.QueueUserPrompt(ctx, "t-wip", "s-wip", "during-wip-wait", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// T2 must NOT drain — the task is in WIP-wait; the admission
	// promotion path is the one that drains.
	if got := svc.messageQueue.GetStatus(ctx, "s-wip").Count; got != 1 {
		t.Fatalf("post-enqueue queue count = %d, want 1 (T2 must not bypass WIP wait)", got)
	}
}

// TestQueueUserPrompt_T2DrainsWhenTaskAdmitted pins the positive
// WIP-admitted case: a task with WIPAdmitted=true (or no
// QueuedForStepID) is ready for prompt, and the fast-path drain
// fires. drainQueuedMessageForPromptableSession reserves the head,
// removing it from the queue (count → 0). The dispatcher's own
// promptTask picks up the queue head but the mock agent cannot
// truly resume — that doesn't change the count==0 invariant.
func TestQueueUserPrompt_T2DrainsWhenTaskAdmitted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// A WIP-admitted task (or no WIP wait at all).
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	// Mark the task as WIP-admitted (seedTaskAndSession defaults to
	// v1.TaskStateInProgress and may not set the WIPAdmitted field).
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WIPAdmitted = true
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "admitted-task", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// T2 should drain immediately — the task is admitted and the
	// session is ready.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("post-enqueue queue count = %d, want 0 (T2 fast-path drained admitted task)", got)
	}
}
