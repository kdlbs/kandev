package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
func TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance(t *testing.T) {
	ctx := context.Background()
	const (
		taskID       = "task-model-switch-before-acceptance"
		sessionID    = "session-model-switch-before-acceptance"
		oldExecution = "execution-model-switch-before-old"
		newExecution = "execution-model-switch-before-new"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session model: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, oldExecution)

	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: newExecution, Status: "starting",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: newExecution}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin model-switch attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID(oldExecution)
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	_, handled, err := svc.trySwitchModelForPrompt(
		ctx, taskID, sessionID, "new-model", "model-switch prompt", session,
		&foregroundDispatch{}, attempt,
	)
	if err != nil {
		t.Fatalf("model-switch fallback: %v", err)
	}
	if !handled {
		t.Fatal("model-switch fallback was not handled")
	}
	manager.mu.Lock()
	onDispatched := manager.initialPromptDispatchCallback
	started := append([]string(nil), manager.startAgentProcessCalls...)
	manager.mu.Unlock()
	if len(started) != 1 || started[0] != newExecution {
		t.Fatalf("StartAgentProcess calls = %#v, want [%q]", started, newExecution)
	}
	if onDispatched == nil {
		t.Fatal("model-switch fallback did not retain the initial-prompt callback")
	}
	// promptTask returns before lifecycle's asynchronous initial prompt callback.
	// Its normal deferred finish must leave the attempt cancellable in this gap.
	attempt.finish(svc.resumeAttemptStore())
	if _, active := svc.resumeAttemptStore().current(sessionID); !active {
		t.Fatal("prompt-task completion removed the pending model-switch attempt before acceptance")
	}

	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	acceptedBeforeCancel := attempt.accepted
	registry.mu.Unlock()
	if acceptedBeforeCancel {
		t.Fatal("startup ownership transferred before the provider accepted the initial prompt")
	}
	svc.invalidateResumeAttempt(sessionID)
	svc.cleanupCancelledResumeAttempt(attempt)

	manager.mu.Lock()
	stops := append([]stopAgentCall(nil), manager.stopAgentWithReasonArgs...)
	manager.mu.Unlock()
	if len(stops) != 1 {
		t.Fatalf("forced cleanup calls = %#v, want one call", stops)
	}
	if stops[0].ExecutionID != newExecution || !stops[0].Force {
		t.Fatalf("forced cleanup call = %#v, want execution %q with force", stops[0], newExecution)
	}

	// A delayed lifecycle callback from the cancelled startup cannot revive
	// provider ownership after exact-execution cleanup has won.
	onDispatched()
	registry.mu.Lock()
	acceptedAfterLateCallback := attempt.accepted
	registry.mu.Unlock()
	if acceptedAfterLateCallback {
		t.Fatal("late initial-prompt callback revived a cancelled resume attempt")
	}
}
