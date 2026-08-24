package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestPromptTask_AdmissionPreventsConcurrentAgentStackReaping(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-prompt-reap-race", "session-prompt-reap-race", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-prompt-reap-race", "task-prompt-reap-race", "exec-prompt-reap-race")

	session, err := repo.GetTaskSession(ctx, "session-prompt-reap-race")
	require.NoError(t, err)
	session.AgentProfileID = "profile-prompt-reap-race"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	lookupEntered := make(chan struct{})
	releaseLookup := make(chan struct{})
	lookupCalls := 0
	stopCalls := make(chan stopAgentCall, 1)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			lookupCalls++
			if lookupCalls == 1 {
				close(lookupEntered)
				<-releaseLookup
			}
			return "exec-prompt-reap-race", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopCalls <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			return nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-prompt-reap-race"] = &v1.Task{
		ID:    "task-prompt-reap-race",
		Title: "Prompt/reaper race",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.config.AgentStackReaping = true
	svc.turnService = &repoTurnService{repo: repo}

	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.PromptTask(
			ctx, "task-prompt-reap-race", "session-prompt-reap-race", "continue", "", false, nil, false,
		)
		promptDone <- promptErr
	}()

	select {
	case <-lookupEntered:
	case <-time.After(time.Second):
		t.Fatal("prompt did not enter the post-ensure admission window")
	}

	require.False(t, svc.stopIdleSessionAgentStack(ctx, session, stopReasonAgentStackIdleTTL))
	assertNoAgentStackStop(t, stopCalls)
	close(releaseLookup)

	select {
	case promptErr := <-promptDone:
		require.NoError(t, promptErr)
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not complete after the staged race was released")
	}
	require.Len(t, agentMgr.capturedPromptCalls, 1)
}
