package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5
func TestSendNowWorkflowTransitionPreservesRunningState(t *testing.T) {
	for _, delivery := range []string{"send_now", "fifo"} {
		t.Run(delivery, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
			seedSession(t, repo, "task", "session", ids["Done"])
			seedExecutorRunning(t, repo, "session", "task", "exec")
			setSessionState(t, ctx, repo, "session", models.TaskSessionStateWaitingForInput)
			eventBus := &recordingEventBus{}
			agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			svc := createEngineService(t, repo, steps, agent)
			svc.eventBus = eventBus
			svc.taskRepo = newTaskServiceStateRepository(repo, eventBus)
			svc.turnService = &repoTurnService{repo: repo}
			svc.messageCreator = &mockMessageCreator{}
			// Inspect state at provider dispatch, before any completion can heal it.
			agent.promptAgentFunc = func(ctx context.Context, _ string, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				session, err := repo.GetTaskSession(ctx, "session")
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateRunning, session.State)
				task, err := repo.GetTask(ctx, "task")
				require.NoError(t, err)
				require.Equal(t, ids["In Progress"], task.WorkflowStepID)
				require.Equal(t, v1.TaskStateInProgress, task.State)
				turn, err := svc.turnService.GetActiveTurn(ctx, "session")
				require.NoError(t, err)
				require.NotNil(t, turn)
				require.Equal(t, "continue", prompt)
				for _, published := range eventBus.events {
					data, err := json.Marshal(published.event.Data)
					require.NoError(t, err)
					var state struct {
						NewState string `json:"new_state"`
					}
					require.NoError(t, json.Unmarshal(data, &state))
					require.NotEqual(t, "WAITING_FOR_INPUT", state.NewState)
					require.NotEqual(t, "REVIEW", state.NewState)
				}
				return &executor.PromptResult{StopReason: "dispatched"}, nil
			}
			queued, err := svc.messageQueue.QueueMessage(ctx, "session", "task", "continue", "", messagequeue.QueuedByUser, false, nil)
			require.NoError(t, err)
			if delivery == "send_now" {
				claim, err := svc.messageQueue.ClaimSendNow(ctx, "session", []messagequeue.QueuedMessage{*queued})
				require.NoError(t, err)
				svc.markQueuedDispatchInFlight("session", claim.Dispatch.ID)
				svc.executeSendNowClaim(claim)
			} else {
				var taken bool
				queued, taken = svc.messageQueue.TakeQueued(ctx, "session")
				require.True(t, taken)
				require.NotNil(t, queued)
				svc.executeQueuedMessage("session", queued)
			}
			require.Len(t, agent.capturedPrompts, 1)
			require.Len(t, svc.messageCreator.(*mockMessageCreator).userMessages, 1)
		})
	}
}

func TestOnTurnStartBeforeAdmissionKeepsSessionPromptable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
	seedSession(t, repo, "task", "session", ids["Done"])
	setSessionState(t, ctx, repo, "session", models.TaskSessionStateWaitingForInput)
	svc := createEngineService(t, repo, steps, &mockAgentManager{repoForExecutionLookup: repo})
	result, err := svc.ProcessOnTurnStart(ctx, "task", "session")
	require.NoError(t, err)
	require.False(t, result.Queued)
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, ids["In Progress"], task.WorkflowStepID)
}
