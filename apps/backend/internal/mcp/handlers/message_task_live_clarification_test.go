package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (f *fakeOrchestrator) SessionHasLiveClarification(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.liveClarifications[sessionID]
}

func TestHandleMessageTask_LiveClarification_QueuesInsteadOfPrompting(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.liveClarifications = map[string]bool{sess.ID: true}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "review report", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "queued", payload["status"])
	assert.Empty(t, orch.promptCalls, "a session waiting on a live question must not be prompted")
	assert.Zero(t, orch.resumeCalls)
	assert.Empty(t, orch.turnStartCalls, "the live question must not run on_turn_start")

	status := orch.queue.GetStatus(ctx, sess.ID)
	require.Equal(t, 1, status.Count)
	require.Len(t, orch.readinessCalls, 1, "queue admission readiness must be rechecked")
	assert.Equal(t, sess.ID, orch.readinessCalls[0].SessionID)
}

func TestHandleMessageTask_NoLiveClarification_PromptsWaitingSession(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.liveClarifications = map[string]bool{"other-session": true}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "next instruction", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
	assert.Zero(t, orch.queue.GetStatus(ctx, sess.ID).Count)
}

func TestHandleMessageTask_LiveClarification_ParentInterruptStillPrompts(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedChildTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sender-sess-1", TaskID: sender.ID, AgentProfileID: "sender-profile",
		State: models.TaskSessionStateRunning,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "child-execution", SessionID: sess.ID, TaskID: target.ID,
		Status: "running", Resumable: true, AgentExecutionID: "child-execution",
	}))
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.liveClarifications = map[string]bool{sess.ID: true}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask,
		senderPayloadWithMode(target.ID, "replacement instruction", sender.ID, "interrupt"))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
	assert.Zero(t, orch.queue.GetStatus(ctx, sess.ID).Count)
}

func TestHandleMessageTask_LauncherWithoutLiveClarificationReaderStillPrompts(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.liveClarifications = map[string]bool{sess.ID: true}
	h.sessionLauncher = struct{ SessionLauncher }{SessionLauncher: orch}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "legacy launcher", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "sent", payload["status"])
	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)
	assert.Zero(t, orch.queue.GetStatus(ctx, sess.ID).Count)
}

func TestHandleMessageTask_LiveClarification_OnSelectedSessionQueues(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, original := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	selected := &models.TaskSession{
		ID: "selected-session", TaskID: target.ID, AgentProfileID: "selected-profile",
		State: models.TaskSessionStateWaitingForInput,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, selected))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "selected-execution", SessionID: selected.ID, TaskID: target.ID,
		Status: "running", Resumable: true, AgentExecutionID: "selected-execution",
	}))
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.liveClarifications = map[string]bool{selected.ID: true}
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		return repo.SetSessionPrimary(ctx, selected.ID)
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "selected peer report", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "queued", payload["status"], "the selected session owns its live question")
	assert.Equal(t, selected.ID, payload["session_id"])
	assert.Empty(t, orch.promptCalls)
	assert.Empty(t, orch.startCreatedCalls, "workflow selection must not supersede the selected session's question")
	require.Equal(t, 1, orch.queue.GetStatus(ctx, selected.ID).Count)
	assert.Zero(t, orch.queue.GetStatus(ctx, original.ID).Count)
	require.Len(t, orch.readinessCalls, 1)
	assert.Equal(t, selected.ID, orch.readinessCalls[0].SessionID)
}

func TestHandleMessageTask_LiveClarification_CompletedTargetPreservesDeliveryMode(t *testing.T) {
	for _, tc := range []struct {
		mode, status    string
		queued, prompts int
	}{
		{mode: "queued", status: "queued", queued: 1},
		{mode: "interrupt", status: "sent", prompts: 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newTestTaskService(t)
			sender, target, sess := seedChildTaskWithSession(t, svc, repo, models.TaskSessionStateCompleted)
			require.NoError(t, repo.UpdateTaskState(ctx, target.ID, v1.TaskStateCompleted))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "sender-sess-1", TaskID: sender.ID, AgentProfileID: "sender-profile",
				State: models.TaskSessionStateRunning,
			}))
			h, orch := newMessageTaskHandler(t, svc, repo)
			resume := completedSessionLaunchFunc(t, repo, target.ID, sess.ID)
			orch.launchFunc = func(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
				result, err := resume(ctx, req)
				require.NoError(t, repo.UpdateTaskState(ctx, target.ID, v1.TaskStateInProgress))
				orch.liveClarifications = map[string]bool{sess.ID: true}
				return result, err
			}
			payload := senderPayloadWithMode(target.ID, "completed target follow-up", sender.ID, tc.mode)
			payload["session_id"] = sess.ID
			resp, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(resp.Payload, &result))
			assert.Equal(t, tc.status, result["status"], "completed resume must preserve delivery mode")
			assert.Equal(t, sess.ID, result["session_id"])
			require.Len(t, orch.launchCalls, 1)
			assert.Len(t, orch.promptCalls, tc.prompts)
			assert.Equal(t, tc.queued, orch.queue.GetStatus(ctx, sess.ID).Count)
			assert.Empty(t, orch.turnStartCalls)
		})
	}
}
