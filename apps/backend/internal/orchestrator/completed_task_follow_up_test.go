package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestCompletedTaskFollowUpAdmissionIsConversationalOnly(t *testing.T) {
	for _, child := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "child"}[child], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
			steps.steps[ids["Done"]].CompleteTaskOnEnter = true
			seedSession(t, repo, "task", "session", ids["New Step"])
			setSessionExecID(t, repo, "session", "exec")
			setSessionState(t, ctx, repo, "session", models.TaskSessionStateRunning)
			if child {
				task, err := repo.GetTask(ctx, "task")
				require.NoError(t, err)
				task.ParentID = "parent"
				require.NoError(t, repo.UpdateTask(ctx, task))
			}

			agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
			svc := createEngineService(t, repo, steps, agentMgr)
			session, err := repo.GetTaskSession(ctx, "session")
			require.NoError(t, err)
			require.True(t, svc.processOnTurnCompleteViaEngine(ctx, "task", session))

			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, task.State)
			require.Equal(t, ids["Done"], task.WorkflowStepID)
			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "session").State)

			result, err := svc.ProcessOnTurnStart(ctx, "task", "session")
			require.NoError(t, err)
			require.False(t, result.Queued)

			followUp, err := repo.GetTaskSession(ctx, "session")
			require.NoError(t, err)
			require.True(t, models.IsCompletionFollowUpSession(followUp.Metadata))
			svc.setSessionRunning(ctx, "task", "session")
			task, err = repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, task.State)

			svc.handleAgentReady(ctx, watcher.AgentEventData{
				TaskID:           "task",
				SessionID:        "session",
				AgentExecutionID: "exec",
			})
			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "session").State)
			task, err = repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, task.State)
			require.Equal(t, ids["Done"], task.WorkflowStepID)
		})
	}
}

func mustGetSession(t *testing.T, repo sessionExecutorStore, sessionID string) *models.TaskSession {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	return session
}
