package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	orchestratorexec "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// drainStopCalls waits for exactly n buffered StopAgentWithReason calls,
// discarding their execution IDs. StopByTaskID schedules each stop on a
// detached goroutine, so a test that does not wait for every call it expects
// leaves that goroutine still in flight when the test returns — caught by
// this package's goleak.VerifyTestMain as a leak even though the send itself
// would have succeeded.
func drainStopCalls(t *testing.T, ch <-chan string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for stop call %d/%d", i+1, n)
		}
	}
}

// TestStopTask_OrphanRecoveryLoadFailureStillReachesReview covers Review
// round 2 Finding 1: a task with one active session that stops cleanly plus
// one registry-only orphan whose row fails to load must still transition the
// task to REVIEW instead of surfacing a false "failed to stop task" to the
// caller, while the load failure stays observable (StopByTaskID's wrapped
// error is still returned to whoever asks for it directly, even though
// StopTask itself does not propagate it as a hard failure). It also covers
// the follow-up fix that the orphan's captured execution is itself targeted
// for stop, not just skipped because its row could not load.
func TestStopTask_OrphanRecoveryLoadFailureStillReachesReview(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-1", "session-active", models.TaskSessionStateRunning)

	loadFailure := errors.New("database is locked")
	hookedRepo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(readCtx context.Context, sessionID string) (*models.TaskSession, error) {
			if sessionID == "session-orphan" {
				return nil, loadFailure
			}
			return baseRepo.GetTaskSession(readCtx, sessionID)
		},
	}

	stopCalls := make(chan string, 8)
	agentManager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{
				{SessionID: "session-active", ExecutionID: "execution-active"},
				{SessionID: "session-orphan", ExecutionID: "execution-orphan"},
			}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-active", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(hookedRepo, taskRepo, agentManager)

	if err := svc.StopTask(ctx, "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopTask returned an error despite the active session stopping cleanly: %v", err)
	}

	// The active session stops normally, and the registry-only orphan is now
	// also targeted by its captured execution ID despite its row failing to
	// load.
	drainStopCalls(t, stopCalls, 2)

	taskRepo.mu.Lock()
	taskState := taskRepo.updatedStates["task-1"]
	taskRepo.mu.Unlock()
	if taskState != v1.TaskStateReview {
		t.Fatalf("task state = %q, want REVIEW despite the orphan load failure", taskState)
	}

	// The load failure itself must still be observable to a direct caller of
	// the executor, so a retry loop built on StopByTaskID does not see a false
	// all-clear.
	directErr := svc.executor.StopByTaskID(ctx, "task-1", "cleanup", true)
	if !errors.Is(directErr, loadFailure) {
		t.Fatalf("StopByTaskID error = %v, want it to still wrap the load failure", directErr)
	}
	if !errors.Is(directErr, orchestratorexec.ErrOrphanRecoveryIncomplete) {
		t.Fatalf("StopByTaskID error = %v, want it to wrap ErrOrphanRecoveryIncomplete", directErr)
	}
	// The retry resolves session-active (now CANCELLED, terminal) through
	// registry recovery instead of the active-state query, plus the
	// still-unloadable orphan; both are targeted again.
	drainStopCalls(t, stopCalls, 2)
}

// TestStopTask_UnloadableOrphanReachesReviewAfterScheduling covers a task
// with no active rows and one unloadable registry orphan. The exact registry
// reference is enough to schedule teardown, so StopTask reaches REVIEW while
// still retaining the load failure for a direct StopByTaskID caller.
func TestStopTask_UnloadableOrphanReachesReviewAfterScheduling(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)

	loadFailure := errors.New("database is locked")
	hookedRepo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
			return nil, loadFailure
		},
	}
	stopCalls := make(chan string, 1)
	agentManager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{{SessionID: "session-orphan", ExecutionID: "execution-orphan"}}
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(hookedRepo, taskRepo, agentManager)

	if err := svc.StopTask(ctx, "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopTask: %v", err)
	}
	select {
	case executionID := <-stopCalls:
		if executionID != "execution-orphan" {
			t.Fatalf("stopped execution = %q, want execution-orphan", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unloadable orphan execution was not scheduled for stop")
	}

	taskRepo.mu.Lock()
	state := taskRepo.updatedStates["task-1"]
	taskRepo.mu.Unlock()
	if state != v1.TaskStateReview {
		t.Fatalf("task state = %q, want REVIEW", state)
	}
}

// TestStopByTaskID_StopsOrphanExecutionWhenSessionRowFailsToLoad covers the
// review finding that a registry-only orphan whose session row cannot be
// loaded must still have its captured execution targeted for stop. A load
// failure on the row must not make the process itself unreachable. Before this
// fix, StopByTaskID skipped the orphan entirely on a load failure and never
// called StopAgentWithReason for it.
func TestStopByTaskID_StopsOrphanExecutionWhenSessionRowFailsToLoad(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)

	loadFailure := errors.New("database is locked")
	hookedRepo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
			return nil, loadFailure
		},
	}

	stopCalls := make(chan string, 1)
	agentManager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{{SessionID: "session-orphan", ExecutionID: "execution-orphan"}}
		},
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			if sessionID != "session-orphan" {
				return "", errors.New("unexpected session ID")
			}
			return "execution-orphan", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(hookedRepo, taskRepo, agentManager)

	err := svc.executor.StopByTaskID(ctx, "task-1", "cleanup", true)
	if !errors.Is(err, loadFailure) {
		t.Fatalf("StopByTaskID error = %v, want it to wrap the load failure", err)
	}
	if !errors.Is(err, orchestratorexec.ErrOrphanRecoveryIncomplete) {
		t.Fatalf("StopByTaskID error = %v, want it to wrap ErrOrphanRecoveryIncomplete since the execution itself was reached", err)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-orphan" {
			t.Fatalf("stopped execution = %q, want execution-orphan", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the registry-only orphan's captured execution was never stopped")
	}
}
