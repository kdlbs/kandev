package handlers

import (
	"context"
	"encoding/json"
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

	createAutomationSender := func(id, automationID, triggerType, policyGeneration string) (*models.Task, string) {
		t.Helper()
		result, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
			WorkspaceID: "ws-1",
			Title:       "Routine sender " + id,
			Origin:      models.TaskOriginAutomationRun,
			IsEphemeral: true,
			Metadata: map[string]interface{}{
				"automation_id":                automationID,
				"automation_name":              "HeartBeat",
				"trigger_id":                   "trigger-" + id,
				"trigger_type":                 triggerType,
				"routine_type":                 "cycle",
				"routine_name":                 "heartbeat",
				"routine_policy_generation":    policyGeneration,
				"routine_scope_generation":     "board-v3",
				"routine_leader_fencing_token": "fence-7",
				"routine_dirty_generation":     "dirty-" + id,
			},
		})
		require.NoError(t, err)
		sessionID := "automation-session-" + id
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: result.Task.ID, State: models.TaskSessionStateRunning,
		}))
		return result.Task, sessionID
	}

	scheduledOne, scheduledSessionOne := createAutomationSender("one", "automation-1", string(automation.TriggerTypeScheduled), "policy-v7")
	scheduledTwo, scheduledSessionTwo := createAutomationSender("two", "automation-2", string(automation.TriggerTypeScheduled), "policy-v7")
	scheduledNextPolicy, scheduledNextPolicySession := createAutomationSender("next-policy", "automation-4", string(automation.TriggerTypeScheduled), "policy-v8")
	webhook, webhookSession := createAutomationSender("webhook", "automation-3", string(automation.TriggerTypeWebhook), "policy-v7")

	sendAutomation := func(sender *models.Task, sessionID, prompt string) *ws.Message {
		t.Helper()
		principalCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
			AutomationID: models.StringFromAny(sender.Metadata["automation_id"]), WorkspaceID: "ws-1",
			CallerTaskID: sender.ID, CallerSessionID: sessionID,
			Surface: mcpprofile.SurfaceAutomation,
		})
		payload := senderPayload(target.ID, prompt, sender.ID)
		payload["sender_session_id"] = sessionID
		resp, err := h.handleMessageTask(principalCtx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		return resp
	}

	sendAutomation(scheduledOne, scheduledSessionOne, "identical routine payload")
	firstStatus := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 1, firstStatus.Count)
	firstID := firstStatus.Entries[0].ID

	coalescedResponse := sendAutomation(scheduledTwo, scheduledSessionTwo, "identical routine payload")
	afterIdentical := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 1, afterIdentical.Count, "identical pending scheduled wakes must coalesce")
	assert.Equal(t, firstID, afterIdentical.Entries[0].ID, "coalescing retains the FIFO entry identity")
	assert.Equal(t, true, afterIdentical.Entries[0].Metadata[messagequeue.MetadataRoutineWake])
	assert.Equal(t, "dirty-two", afterIdentical.Entries[0].Metadata[messagequeue.MetadataRoutineDirtyGeneration])
	receipt := messagequeue.RoutineWakeReceiptFromMessage(&afterIdentical.Entries[0])
	require.NotNil(t, receipt)
	assert.Equal(t, 1, receipt.AbsorbedCount)
	assert.NotEmpty(t, receipt.AbsorbedSources[0].ID)
	var responseBody struct {
		RoutineWake messagequeue.RoutineWakeReceipt `json:"routine_wake"`
	}
	require.NoError(t, json.Unmarshal(coalescedResponse.Payload, &responseBody))
	assert.Equal(t, firstID, responseBody.RoutineWake.CanonicalEntryID)
	assert.Equal(t, 1, responseBody.RoutineWake.AbsorbedCount)
	assert.NotEmpty(t, responseBody.RoutineWake.AbsorbedSources[0].QueuedAt)

	ordinaryPayload := senderPayload(target.ID, "identical routine payload", ordinarySender.ID)
	resp, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, ordinaryPayload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	sendAutomation(scheduledTwo, scheduledSessionTwo, "materially different routine payload")
	sendAutomation(scheduledNextPolicy, scheduledNextPolicySession, "identical routine payload")
	sendAutomation(webhook, webhookSession, "identical routine payload")

	status := orch.queue.GetStatus(ctx, targetSession.ID)
	require.Equal(t, 5, status.Count)
	assert.Equal(t, firstID, status.Entries[0].ID, "routine replacement must retain FIFO position")
	assert.Contains(t, status.Entries[1].Content, "identical routine payload")
	assert.Contains(t, status.Entries[2].Content, "materially different routine payload")
	assert.Contains(t, status.Entries[3].Content, "identical routine payload")
	assert.Contains(t, status.Entries[4].Content, "identical routine payload")
	assert.NotEqual(t, true, status.Entries[1].Metadata[messagequeue.MetadataRoutineWake], "ordinary peer message must not be routine-coalesced")
	assert.Equal(t, true, status.Entries[3].Metadata[messagequeue.MetadataRoutineWake], "different policy generations stay distinct routine entries")
	assert.NotEqual(t, true, status.Entries[4].Metadata[messagequeue.MetadataRoutineWake], "event automation message must not be routine-coalesced")
}

func TestHandleMessageTask_RoutineWakeFailsClosedOutsideCurrentWorkspaceScope(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, targetSession := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}))
	senderResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-2",
		Title:       "Foreign HeartBeat",
		Origin:      models.TaskOriginAutomationRun,
		IsEphemeral: true,
		Metadata: map[string]interface{}{
			"automation_id":   "automation-foreign",
			"automation_name": "HeartBeat",
			"trigger_id":      "trigger-foreign",
			"trigger_type":    string(automation.TriggerTypeScheduled),
		},
	})
	require.NoError(t, err)
	senderSessionID := "foreign-automation-session"
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: senderSessionID, TaskID: senderResult.Task.ID, State: models.TaskSessionStateRunning,
	}))
	h, orch := newMessageTaskHandler(t, svc)
	principalCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "automation-foreign", WorkspaceID: "ws-2",
		CallerTaskID: senderResult.Task.ID, CallerSessionID: senderSessionID,
		Surface: mcpprofile.SurfaceAutomation,
	})
	payload := senderPayload(target.ID, "WAKE:CYCLE", senderResult.Task.ID)
	payload["sender_session_id"] = senderSessionID

	resp, err := h.handleMessageTask(principalCtx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	assert.Equal(t, 0, orch.queue.GetStatus(ctx, targetSession.ID).Count)
}

func TestHandleMessageTask_RoutineWakeRejectsExplicitNonPrimaryTarget(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	ctx := context.Background()
	senderResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "HeartBeat", Origin: models.TaskOriginAutomationRun, IsEphemeral: true,
		Metadata: map[string]interface{}{
			"automation_id": "automation-1", "automation_name": "HeartBeat",
			"trigger_id": "trigger-1", "trigger_type": string(automation.TriggerTypeScheduled),
		},
	})
	require.NoError(t, err)
	senderSessionID := "routine-sender-session"
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: senderSessionID, TaskID: senderResult.Task.ID, State: models.TaskSessionStateRunning,
	}))
	nonPrimary := &models.TaskSession{
		ID: "stale-target-session", TaskID: target.ID, State: models.TaskSessionStateRunning, IsPrimary: false,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, nonPrimary))
	h, orch := newMessageTaskHandler(t, svc)
	principalCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "automation-1", WorkspaceID: "ws-1",
		CallerTaskID: senderResult.Task.ID, CallerSessionID: senderSessionID,
		Surface: mcpprofile.SurfaceAutomation,
	})
	payload := senderPayload(target.ID, "WAKE:CYCLE", senderResult.Task.ID)
	payload["sender_session_id"] = senderSessionID
	payload["session_id"] = nonPrimary.ID

	resp, err := h.handleMessageTask(principalCtx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	assert.Equal(t, 0, orch.queue.GetStatus(ctx, nonPrimary.ID).Count)
}
