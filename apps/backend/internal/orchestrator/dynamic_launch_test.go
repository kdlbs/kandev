package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRelaunchDynamicTaskAfterFailure_DoesNotLaunchSuccessorWhenStopFails(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-stop-failure"
		sessionID   = "session-dynamic-stop-failure"
		executionID = "execution-dynamic-stop-failure"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	stopErr := errors.New("runtime teardown failed")
	agentManager := &mockAgentManager{
		stopAgentWithReasonErr: stopErr,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	relaunched := svc.relaunchDynamicTaskAfterFailure(
		ctx,
		watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
		},
		"fallback-profile",
	)

	if relaunched {
		t.Fatal("relaunchDynamicTaskAfterFailure returned success after predecessor stop failed")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING while predecessor teardown is unresolved", session.State)
	}
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after stop failure", len(agentManager.startAgentProcessCalls))
	}
	if len(agentManager.stopAgentWithReasonArgs) != 1 {
		t.Fatalf("stop calls = %d, want 1", len(agentManager.stopAgentWithReasonArgs))
	}
	if agentManager.stopAgentWithReasonArgs[0] != (stopAgentCall{
		ExecutionID: executionID,
		Reason:      "dynamic route fallback",
		Force:       true,
	}) {
		t.Fatalf("unexpected stop call: %#v", agentManager.stopAgentWithReasonArgs[0])
	}
}
