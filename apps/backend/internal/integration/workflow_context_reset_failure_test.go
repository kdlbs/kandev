package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWorkflowResetEscalationLeavesSessionDeletable(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	taskID := ts.CreateTestTask(t, "augment-agent", 2)
	task, err := ts.TaskRepo.GetTask(ctx, taskID)
	require.NoError(t, err)

	steps, err := ts.WorkflowSvc.ListStepsByWorkflow(ctx, task.WorkflowID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(steps), 2)
	var target *wfmodels.WorkflowStep
	for _, step := range steps {
		if step.ID != task.WorkflowStepID {
			target = step
			break
		}
	}
	require.NotNil(t, target)
	target.Prompt = "distinct reset failure workflow prompt"
	target.Events.OnEnter = []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterResetAgentContext},
		{Type: wfmodels.OnEnterAutoStartAgent},
	}
	require.NoError(t, ts.WorkflowSvc.UpdateStep(ctx, target))

	// Keep the initial simulated turn active while the workflow move runs.
	ts.AgentManager.SetExecutionTime(30 * time.Second)
	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	startResp, err := client.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, ok := startPayload["session_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, sessionID)

	// StartTask creates the active turn before the launch response. Wait for
	// the durable state so the move exercises reset's active-turn admission.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, getErr := ts.TaskRepo.GetTaskSession(ctx, sessionID)
		if getErr == nil && session.State == models.TaskSessionStateRunning {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	session, err := ts.TaskRepo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	promptCallsBeforeMove := ts.AgentManager.PromptCallCount()
	messagesBeforeMove, err := ts.TaskRepo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	userMessagesBeforeMove := countUserMessages(messagesBeforeMove)

	ts.AgentManager.MarkAgentHungForSession(sessionID)
	launchCountBeforeMove := ts.AgentManager.GetLaunchCount()
	moveResp, err := client.SendRequest("move-1", ws.ActionTaskMove, map[string]interface{}{
		"id":               taskID,
		"workflow_id":      task.WorkflowID,
		"workflow_step_id": target.ID,
		"position":         target.Position,
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, moveResp.Type)

	var lastError models.LastAgentError
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, getErr := ts.TaskRepo.GetTaskSession(ctx, sessionID)
		if getErr == nil {
			if candidate, found := models.LoadLastAgentError(session.Metadata); found {
				lastError = candidate
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.NotEmpty(t, lastError.Message)
	require.Contains(t, strings.ToLower(lastError.Message), "context reset")
	require.Contains(t, strings.ToLower(lastError.Message), "step prompt")
	require.Contains(t, strings.ToLower(lastError.Details), "provider cancellation escalated")
	require.Equal(t, 0, ts.AgentManager.ResetContextCallCount())
	require.Equal(t, 0, ts.AgentManager.RestartProcessCallCount())
	require.Equal(t, launchCountBeforeMove, ts.AgentManager.GetLaunchCount())
	require.Equal(t, promptCallsBeforeMove, ts.AgentManager.PromptCallCount())
	messagesAfterMove, err := ts.TaskRepo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, userMessagesBeforeMove, countUserMessages(messagesAfterMove))

	deleteResp, err := client.SendRequest("delete-1", ws.ActionSessionDelete, map[string]interface{}{
		"session_id": sessionID,
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, deleteResp.Type)
	_, err = ts.TaskRepo.GetTaskSession(ctx, sessionID)
	require.Error(t, err)
	require.True(t, errors.Is(err, models.ErrTaskSessionNotFound), "session delete error = %v", err)
}

func countUserMessages(messages []*models.Message) int {
	count := 0
	for _, message := range messages {
		if message != nil && message.AuthorType == models.MessageAuthorUser {
			count++
		}
	}
	return count
}
