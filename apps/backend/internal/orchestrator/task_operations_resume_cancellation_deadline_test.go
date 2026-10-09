package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCancelAgentFinalizesSessionAfterStartupStopOutlivesOperationContext(t *testing.T) {
	const (
		taskID      = "task-cancel-resume-deadline"
		sessionID   = "session-cancel-resume-deadline"
		executionID = "execution-cancel-resume-deadline"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	var releaseStopOnce sync.Once
	unblockStop := func() { releaseStopOnce.Do(func() { close(releaseStop) }) }
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			close(stopStarted)
			<-releaseStop
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	attempt, owner, err := svc.beginResumeAttempt(context.Background(), taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin resume attempt: owner=%v err=%v", owner, err)
	}
	attempt.setExecutionID(executionID)
	t.Cleanup(func() {
		unblockStop()
		attempt.finish(svc.resumeAttemptStore())
	})

	operation, owner, _ := svc.claimExplicitCancellation(sessionID, nil)
	if !owner {
		t.Fatal("explicit cancellation was not admitted as the owner")
	}
	operationCtx, cancelOperation := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		operationErr := svc.runExplicitCancellationOwned(operationCtx, sessionID, operation)
		svc.finishCancellationWithActions(context.Background(), sessionID, operation, operationErr)
		done <- operationErr
	}()

	select {
	case <-stopStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("startup execution stop did not begin")
	}
	cancelOperation()
	unblockStop()

	if err := <-done; err != nil {
		t.Fatalf("cancel startup after operation context expired: %v", err)
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
}
