package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	orchestratorexec "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestStopTask_OrphanRecoveryLoadFailureStillReachesReview covers Review
// round 2 Finding 1: a task with one active session that stops cleanly plus
// one registry-only orphan whose row fails to load must still transition the
// task to REVIEW instead of surfacing a false "failed to stop task" to the
// caller, while the load failure stays observable (StopByTaskID's wrapped
// error is still returned to whoever asks for it directly, even though
// StopTask itself does not propagate it as a hard failure).
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

	stopCalls := make(chan string, 2)
	agentManager := &mockAgentManager{
		listSessionIDsForTaskFunc: func(taskID string) []string {
			if taskID != "task-1" {
				return nil
			}
			return []string{"session-active", "session-orphan"}
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

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-active" {
			t.Fatalf("stopped execution = %q, want execution-active", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active session's execution was not stopped despite the orphan load failure")
	}

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
}

// TestStopTask_NothingStoppedOrphanLoadFailureStillFails covers the other
// half of Finding 1's synthesis: when nothing at all stopped (no active
// session, only an unloadable orphan), StopTask must still return a hard
// error so a caller with nothing to show for the attempt does not silently
// reach REVIEW.
func TestStopTask_NothingStoppedOrphanLoadFailureStillFails(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)

	loadFailure := errors.New("database is locked")
	hookedRepo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
			return nil, loadFailure
		},
	}
	agentManager := &mockAgentManager{
		listSessionIDsForTaskFunc: func(taskID string) []string {
			if taskID != "task-1" {
				return nil
			}
			return []string{"session-orphan"}
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(hookedRepo, taskRepo, agentManager)

	err := svc.StopTask(ctx, "task-1", "cleanup", true)
	if err == nil {
		t.Fatal("StopTask: want an error when nothing could be stopped, got nil")
	}
	if !errors.Is(err, loadFailure) {
		t.Fatalf("StopTask error = %v, want it to wrap the load failure", err)
	}

	taskRepo.mu.Lock()
	_, wasWritten := taskRepo.updatedStates["task-1"]
	taskRepo.mu.Unlock()
	if wasWritten {
		t.Fatal("task state was updated to REVIEW despite nothing being stopped")
	}
}
