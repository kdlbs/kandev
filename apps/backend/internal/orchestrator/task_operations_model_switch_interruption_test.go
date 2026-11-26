package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestWarmModelSwitchReplacementResolvesInterruptionOnCurrentAcceptance(t *testing.T) {
	ctx := context.Background()
	const (
		taskID               = "task-warm-switch-interruption"
		sessionID            = "session-warm-switch-interruption"
		oldExecution         = "execution-warm-switch-old"
		replacementExecution = "execution-warm-switch-replacement"
		successorExecution   = "execution-warm-switch-successor"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, sessionID, taskID, oldExecution)
	manager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: replacementExecution, Status: models.ExecutorRunningStatusStarting,
			}))
			return &executor.LaunchAgentResponse{AgentExecutionID: replacementExecution}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = withTestCapabilities(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
	svc.turnService = &repoTurnService{repo: repo}
	marker := addInterruptedSessionMarker(t, repo, taskID, session)

	_, err = svc.promptTask(
		ctx, taskID, sessionID, "switch model and continue", "new-model", false, nil,
		false, launchOriginManual, promptTaskOptions{},
	)
	require.NoError(t, err)
	manager.mu.Lock()
	onAccepted := manager.initialPromptDispatchCallback
	manager.mu.Unlock()
	require.NotNil(t, onAccepted, "the restart replacement must own initial-prompt acceptance even without a resume attempt")

	seedExecutorRunning(t, repo, sessionID, taskID, successorExecution)
	onAccepted()
	requireTaskInterruptionEntries(t, repo, taskID, map[string]models.InterruptedSessionMarker{sessionID: marker})

	seedExecutorRunning(t, repo, sessionID, taskID, replacementExecution)
	onAccepted()
	requireTaskInterruptionEntries(t, repo, taskID, nil)
}
