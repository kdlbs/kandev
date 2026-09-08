package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestQueueUserPrompt_RuntimeUnavailableSurvivesFastPathRedispatch is the
// regression test for Review round 1's F1 finding: QueueUserPrompt's T2
// fast-path drain (tryFastPathDrainAfterEnqueue) reloads the session with
// only checkSessionPromptable, which cannot tell "promptable" apart from
// "promptable but its runtime has not finished launching". So the fast path
// takes the entry and re-dispatches it through promptTask microseconds
// after it was queued — with the runtime exactly as unavailable as it was
// the first time.
//
// This drives the real Service.QueueUserPrompt -> tryFastPathDrainAfterEnqueue
// -> executeQueuedMessageWithReservation -> promptTask -> handleQueuedMessageExecutionError
// chain end to end (no fakes), the way F3 requires: a fake orchestrator that
// only counts QueueUserPrompt calls cannot see this loop at all, because the
// loop happens entirely inside the real Service after the initial enqueue
// returns.
func TestQueueUserPrompt_RuntimeUnavailableSurvivesFastPathRedispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// WAITING_FOR_INPUT with no executors_running row and no in-memory
	// execution reproduces the exact production shape: a session promoted
	// by a workflow step move whose runtime has not launched yet.
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})

	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "hello", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the fast-path redispatch to settle")
	}

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "s1")
	if err != nil {
		t.Fatalf("snapshot queue: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("message lost: fast-path redispatch failed with ErrSessionRuntimeUnavailable and "+
			"was not requeued (queue has %d entries, want 1)", len(entries))
	}
	if entries[0].Content != "hello" {
		t.Fatalf("surviving entry content = %q, want %q", entries[0].Content, "hello")
	}
}

// TestPromptTask_SessionAwaitingLaunchClassifiedRuntimeUnavailable pins the
// positive case for the narrowed sentinel (Review round 1's F2 finding): the
// exact "prepared but not launched yet" shape (WAITING_FOR_INPUT, no
// executors_running row) must still classify as ErrSessionRuntimeUnavailable
// so the caller queues instead of surfacing an error.
func TestPromptTask_SessionAwaitingLaunchClassifiedRuntimeUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})

	_, err := svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected an error: no executors_running row exists for this session")
	}
	if !errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("expected ErrSessionRuntimeUnavailable for the not-yet-launched shape, got: %v", err)
	}
}

// TestPromptTask_ExhaustedColdResumeNotClassifiedRuntimeUnavailable pins the
// negative case for Review round 1's F2 finding: a session that has an
// executors_running row (so a resume attempt actually runs) and never
// becomes prompt-ready is a genuine launch failure, not the "hasn't started
// yet" case ErrSessionRuntimeUnavailable exists for. It must keep surfacing
// as a plain, visible error so it isn't silently queued forever with no
// error row and no log — the exact regression F2 flagged.
func TestPromptTask_ExhaustedColdResumeNotClassifiedRuntimeUnavailable(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er1",
		SessionID:        "s1",
		TaskID:           "t1",
		AgentExecutionID: "exec-wedged",
		Status:           "running",
		Resumable:        true,
		ResumeToken:      "resume-token-123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn:       func(context.Context, string) bool { return false },
		isAgentReadyFn:         func(context.Context, string) bool { return false },
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			return nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			go func(sessID string) {
				sess, gErr := repo.GetTaskSession(ctx, sessID)
				if gErr == nil && sess != nil {
					sess.State = models.TaskSessionStateWaitingForInput
					sess.UpdatedAt = time.Now().UTC()
					_ = repo.UpdateTaskSession(ctx, sess)
				}
			}(req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-still-wedged"}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected the exhausted cold-resume retry to fail")
	}
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("a genuine resume failure must not classify as ErrSessionRuntimeUnavailable "+
			"(it would be silently queued with no visible error): %v", err)
	}
}

// executorLookupErrorRepo wraps the real repo but forces
// GetExecutorRunningBySessionID to fail with a non-not-found error, standing
// in for a transient DB/context/deserialization failure.
type executorLookupErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *executorLookupErrorRepo) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	return nil, r.err
}

// TestPromptTask_ExecutorLookupErrorNotClassifiedRuntimeUnavailable pins a
// second negative case for Review round 1's F2 finding, raised again in PR
// fixup review: a genuine executor-lookup failure (DB error, context
// cancellation, metadata deserialization) is not the "no row yet" shape
// errSessionAwaitingRuntimeLaunch exists for. It must stay visible instead of
// being classified as ErrSessionRuntimeUnavailable and silently queued with
// no future agent.boot_ready to drain it.
func TestPromptTask_ExecutorLookupErrorNotClassifiedRuntimeUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &executorLookupErrorRepo{Repository: repo, err: errors.New("db unavailable")}

	_, err := svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected a visible error for a genuine executor lookup failure")
	}
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("a genuine executor lookup error must not classify as ErrSessionRuntimeUnavailable "+
			"(it would be silently queued with no future drain trigger): %v", err)
	}
}
