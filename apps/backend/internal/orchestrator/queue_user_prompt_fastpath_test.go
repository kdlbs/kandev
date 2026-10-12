package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type fastPathTaskReadRetryRepo struct {
	*sqliterepo.Repository
	failNext atomic.Bool
}

func newFastPathDispatchService(t *testing.T, repo *sqliterepo.Repository) *Service {
	t.Helper()
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	agentManager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }
	t.Cleanup(func() {
		select {
		case <-workerDone:
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for fast-path dispatch worker")
		}
	})
	return svc
}

func (r *fastPathTaskReadRetryRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	if r.failNext.CompareAndSwap(true, false) {
		return nil, errors.New("transient task read failure")
	}
	return r.Repository.GetTask(ctx, taskID)
}

func TestQueueUserPromptRejectsTerminalSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	err := svc.QueueUserPrompt(ctx, "t1", "s1", "late prompt", "", false, nil, nil, true)
	if !errors.Is(err, ErrSessionNotPromptable) {
		t.Fatalf("QueueUserPrompt error = %v, want ErrSessionNotPromptable", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("terminal session queue count = %d, want 0", got)
	}
}

// TestQueueUserPrompt_T2FastPathDrainsPromptableSession pins the
// T2 contract: a user message admitted via QueueUserPrompt triggers
// a synchronous fast-path drain when the session is ready for input,
// no in-flight cancellation/queue/steer holds the guard, and the task
// is not in WIP-wait. The drain uses the existing public helper
// drainQueuedMessageForPromptableSession.
//
// ReserveQueued removes the head from the queue, so count drops to 0.
// The mock dispatch worker completes before fixture teardown.
func TestQueueUserPrompt_T2FastPathDrainsPromptableSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := newFastPathDispatchService(t, repo)

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("pre-condition: queue count = %d, want 0", got)
	}
	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "hello", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// drainQueuedMessageForPromptableSession reserves the head before
	// dispatching. The reservation removes the entry from the queue,
	// so the count drops to 0 once T2's fast-path drain is reached.
	// The downstream dispatch may fail, but the count==0 invariant pins the T2
	// call site and the fixture waits for the worker before teardown.
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
	svc := newFastPathDispatchService(t, repo)

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
	svc := newFastPathDispatchService(t, repo)
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

// TestQueueUserPrompt_T2InitialTaskBriefContenderOwnership pins both sides of
// the first-prompt ordering boundary through QueueUserPrompt and the real
// queue dispatcher. A contender waits while the winner owns dispatch, then a
// contender arriving after completion drains immediately.
func TestQueueUserPrompt_T2InitialTaskBriefContenderOwnership(t *testing.T) {
	ctx := context.Background()
	metadata := map[string]interface{}{MetaKeyInitialTaskBriefDispatchPending: true}

	t.Run("queued before winner dispatch completes", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
		svc := newFastPathDispatchService(t, repo)
		ownerEntered := make(chan struct{})
		releaseOwnerAdmission := make(chan struct{})
		var releaseOwnerOnce sync.Once
		releaseOwner := func() { releaseOwnerOnce.Do(func() { close(releaseOwnerAdmission) }) }
		defer releaseOwner()
		ownerAdmissionDone := make(chan error, 1)
		go func() {
			ownerAdmissionDone <- svc.WithInitialTaskBriefAdmission(ctx, "s1", func(context.Context) error {
				svc.MarkInitialTaskBriefDispatchPending("s1")
				close(ownerEntered)
				<-releaseOwnerAdmission
				return nil
			})
		}()
		<-ownerEntered

		queued := make(chan error, 1)
		go func() {
			queued <- svc.QueueUserPrompt(ctx, "t1", "s1", "before owner completion", "", false, nil, metadata, true)
		}()
		select {
		case err := <-queued:
			t.Fatalf("contender crossed the held first-boundary admission: %v", err)
		default:
		}
		releaseOwner()
		require.NoError(t, <-ownerAdmissionDone)
		require.NoError(t, <-queued)
		require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "s1").Count)
		require.False(t, svc.isQueuedDispatchInFlight("s1"), "contender must not reserve ahead of the pending owner")

		svc.CompleteInitialTaskBriefDispatch(ctx, "t1", "s1")
		require.Eventually(t, func() bool { return svc.messageQueue.GetStatus(ctx, "s1").Count == 0 }, 5*time.Second, 10*time.Millisecond)
	})

	t.Run("queued after winner completion", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
		svc := newFastPathDispatchService(t, repo)
		require.NoError(t, svc.WithInitialTaskBriefAdmission(ctx, "s1", func(context.Context) error {
			svc.MarkInitialTaskBriefDispatchPending("s1")
			return nil
		}))
		svc.CompleteInitialTaskBriefDispatch(ctx, "t1", "s1")

		require.NoError(t, svc.QueueUserPrompt(
			ctx, "t1", "s1", "after owner completion", "", false, nil, metadata, true,
		))
		require.Eventually(t, func() bool { return svc.messageQueue.GetStatus(ctx, "s1").Count == 0 }, 5*time.Second, 10*time.Millisecond)
	})
}

func TestPromptTaskRejectsNonOwnerWhileInitialTaskBriefDispatchIsPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.NoError(t, svc.WithInitialTaskBriefAdmission(ctx, "s1", func(context.Context) error {
		svc.MarkInitialTaskBriefDispatchPending("s1")
		return nil
	}))

	_, err := svc.PromptTask(ctx, "t1", "s1", "follow-up", "", false, nil, false)
	require.ErrorIs(t, err, ErrInitialTaskBriefDispatchPending)
}

func TestNotifyQueuedUserPromptDefersDrainWhileInitialTaskBriefIsPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := newFastPathDispatchService(t, repo)
	require.NoError(t, svc.WithInitialTaskBriefAdmission(ctx, "s1", func(context.Context) error {
		svc.MarkInitialTaskBriefDispatchPending("s1")
		return nil
	}))

	require.NoError(t, svc.QueueUserPrompt(
		ctx, "t1", "s1", "follow-up", "", false, nil,
		map[string]interface{}{MetaKeyInitialTaskBriefDispatchPending: true}, true,
	))
	svc.NotifyQueuedUserPrompt(ctx, "t1", "s1")
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("queued follow-up count during owner dispatch = %d, want 1", got)
	}

	svc.CompleteInitialTaskBriefDispatch(ctx, "t1", "s1")
	require.Eventually(t, func() bool { return svc.messageQueue.GetStatus(ctx, "s1").Count == 0 }, 5*time.Second, 10*time.Millisecond)
}

func TestLifecyclePromptDefersDrainWhileInitialTaskBriefIsPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := newFastPathDispatchService(t, repo)
	require.NoError(t, svc.WithInitialTaskBriefAdmission(ctx, "s1", func(context.Context) error {
		svc.MarkInitialTaskBriefDispatchPending("s1")
		return nil
	}))
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	_, err = svc.queueAndDrainLifecyclePrompt(
		ctx, session, "t1", "lifecycle feedback", nil, "brief-pending-lifecycle", errors.New("inactive"),
	)
	require.NoError(t, err)
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("lifecycle queue count during owner dispatch = %d, want 1", got)
	}

	svc.CompleteInitialTaskBriefDispatch(ctx, "t1", "s1")
	require.Eventually(t, func() bool { return svc.messageQueue.GetStatus(ctx, "s1").Count == 0 }, 5*time.Second, 10*time.Millisecond)
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
	seedExecutorRunning(t, repo, "s1", "t1", "exec-fastpath-admitted")
	promptCalled := make(chan struct{})
	manager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			close(promptCalled)
			return nil, errors.New("test prompt delivery failure")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "admitted-task", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	// T2 should drain immediately — the task is admitted and the
	// session is ready.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("post-enqueue queue count = %d, want 0 (T2 fast-path drained admitted task)", got)
	}
	select {
	case <-promptCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("fast-path queue drain did not reach the mock provider")
	}
	require.Eventually(t, func() bool {
		return !svc.isQueuedDispatchInFlight("s1")
	}, 5*time.Second, 10*time.Millisecond, "fast-path dispatch did not settle before test cleanup")
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("fast-path dispatch worker did not finish before test cleanup")
	}
}
