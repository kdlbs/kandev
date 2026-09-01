package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/automation"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMessageTask_CoalescesOnlyIdenticalScheduledRoutineWakes(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ordinarySender, target, targetSession := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc)
	ctx := context.Background()

	createAutomationSender := func(id, triggerType string) (*models.Task, string) {
		t.Helper()
		result, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
			WorkspaceID: "ws-1",
			Title:       "Routine sender " + id,
			Origin:      models.TaskOriginAutomationRun,
			IsEphemeral: true,
			Metadata: map[string]interface{}{
				"automation_id": "automation-1",
				"trigger_id":    "trigger-1",
				"trigger_type":  triggerType,
			},
		})
		require.NoError(t, err)
		sessionID := "automation-session-" + id
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: result.Task.ID, State: models.TaskSessionStateRunning,
		}))
		return result.Task, sessionID
	}

	scheduledOne, scheduledSessionOne := createAutomationSender("one", string(automation.TriggerTypeScheduled))
	scheduledTwo, scheduledSessionTwo := createAutomationSender("two", string(automation.TriggerTypeScheduled))
	webhook, webhookSession := createAutomationSender("webhook", string(automation.TriggerTypeWebhook))

	sendAutomation := func(sender *models.Task, sessionID, prompt string) {
		t.Helper()
		principalCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
			AutomationID: "automation-1", WorkspaceID: "ws-1",
			CallerTaskID: sender.ID, CallerSessionID: sessionID,
			Surface: mcpprofile.SurfaceAutomation,
		})
		payload := senderPayload(target.ID, prompt, sender.ID)
		payload["sender_session_id"] = sessionID
		resp, err := h.handleMessageTask(principalCtx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
	}

	sendAutomation(scheduledOne, scheduledSessionOne, "identical routine payload")
	firstStatus := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 1, firstStatus.Count)
	firstID := firstStatus.Entries[0].ID

	sendAutomation(scheduledTwo, scheduledSessionTwo, "identical routine payload")
	afterIdentical := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 1, afterIdentical.Count, "identical pending scheduled wakes must coalesce")
	assert.Equal(t, firstID, afterIdentical.Entries[0].ID, "coalescing retains the FIFO entry identity")
	assert.Equal(t, true, afterIdentical.Entries[0].Metadata[messagequeue.MetadataRoutineWake])

	ordinaryPayload := senderPayload(target.ID, "identical routine payload", ordinarySender.ID)
	resp, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, ordinaryPayload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	sendAutomation(scheduledTwo, scheduledSessionTwo, "materially different routine payload")
	sendAutomation(webhook, webhookSession, "identical routine payload")

	status := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 4, status.Count)
	assert.Equal(t, firstID, status.Entries[0].ID, "routine replacement must retain FIFO position")
	assert.Contains(t, status.Entries[1].Content, "identical routine payload")
	assert.Contains(t, status.Entries[2].Content, "materially different routine payload")
	assert.Contains(t, status.Entries[3].Content, "identical routine payload")
	assert.NotEqual(t, true, status.Entries[1].Metadata[messagequeue.MetadataRoutineWake], "ordinary peer message must not be routine-coalesced")
	assert.NotEqual(t, true, status.Entries[3].Metadata[messagequeue.MetadataRoutineWake], "event automation message must not be routine-coalesced")
}
