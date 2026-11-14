package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func sendOperationTestContext(taskID, sessionID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CallerTaskID: taskID, CallerSessionID: sessionID, Surface: mcpprofile.SurfaceKanbanTask,
	})
}

func TestMessageTaskOperationReplayReturnsReceiptWithoutDispatch(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	h.SetSendOperationStore(repo)
	ctx := sendOperationTestContext(sender.ID, "sender-sess-1")
	payload := senderPayload(target.ID, "deliver this once", sender.ID)
	payload[sendOperationIDParam] = "operation-once"

	first, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type)
	require.Len(t, orch.promptCalls, 1)

	var firstReceipt map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Payload, &firstReceipt))
	require.Equal(t, models.SendOperationStateCommitted, firstReceipt[sendOperationStateKey])
	require.NotEmpty(t, firstReceipt[sendOperationMessageKey])

	replay, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, replay.Type)
	require.Len(t, orch.promptCalls, 1, "a replay must not dispatch another prompt")

	var replayReceipt map[string]interface{}
	require.NoError(t, json.Unmarshal(replay.Payload, &replayReceipt))
	require.Equal(t, true, replayReceipt["replayed"])
	require.Equal(t, models.SendOperationStateCommitted, replayReceipt[sendOperationStateKey])
	require.Equal(t, firstReceipt[sendOperationMessageKey], replayReceipt[sendOperationMessageKey])
	messages, err := svc.ListMessages(context.Background(), session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1, "a replay must not create a second task message")
}

func TestMessageTaskOperationRejectsConflictingReplay(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	h.SetSendOperationStore(repo)
	ctx := sendOperationTestContext(sender.ID, "sender-sess-1")
	payload := senderPayload(target.ID, "original request", sender.ID)
	payload[sendOperationIDParam] = "operation-conflict"

	_, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	require.Len(t, orch.promptCalls, 1)

	payload["prompt"] = "changed request"
	conflict, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	assertWSError(t, conflict, sendOperationErrorConflict)
	require.Len(t, orch.promptCalls, 1, "a conflicting replay must not dispatch")
}

func TestMessageTaskOperationRequiresAuthorizedSender(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)
	h.SetSendOperationStore(repo)
	payload := senderPayload(target.ID, "do not send", sender.ID)
	payload[sendOperationIDParam] = "operation-unauthorized"

	resp, err := h.handleMessageTask(sendOperationTestContext(sender.ID, "different-session"),
		makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Empty(t, orch.promptCalls)
	if _, err := repo.GetSendOperation(context.Background(), sender.ID, "sender-sess-1", "operation-unauthorized"); !errors.Is(err, models.ErrSendOperationNotFound) {
		t.Fatalf("unauthorized operation lookup error = %v, want ErrSendOperationNotFound", err)
	}
}

func TestGetTaskMessageOperationReadbackReportsSafeRetryState(t *testing.T) {
	_, repo := newTestTaskService(t)
	h := &Handlers{logger: testLogger(t)}
	h.SetSendOperationStore(repo)
	const (
		taskID    = "sender-task"
		sessionID = "sender-session"
	)
	tests := []struct {
		name        string
		state       string
		failureCode string
		retrySafe   bool
	}{
		{name: "pending", state: models.SendOperationStatePending},
		{name: "committed", state: models.SendOperationStateCommitted},
		{name: "unknown failure", state: models.SendOperationStateFailed, failureCode: models.SendOperationFailureOutcomeUnknown},
		{name: "definite rejection", state: models.SendOperationStateFailed, failureCode: models.SendOperationFailureDispatchRejected, retrySafe: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operationID := "readback-" + strings.ReplaceAll(test.name, " ", "-")
			operation := &models.TaskMessageSendOperation{
				ID: "row-" + operationID, OperationID: operationID,
				SenderTaskID: taskID, SenderSessionID: sessionID,
				TargetTaskID: "target-task", TargetSessionID: "target-session",
				RequestedPrompt: "private prompt contents", CreatedAt: time.Now().UTC(),
			}
			require.NoError(t, repo.ClaimSendOperation(context.Background(), operation))
			if test.state != models.SendOperationStatePending {
				_, err := repo.SettleSendOperation(context.Background(), taskID, sessionID, operationID,
					test.state, "target-session", "", "", "", test.failureCode)
				require.NoError(t, err)
			}

			resp, err := h.handleGetTaskMessageOperation(sendOperationTestContext(taskID, sessionID),
				makeWSMessage(t, ws.ActionMCPGetTaskMessageOperation, map[string]interface{}{
					"task_id": taskID, "sender_task_id": taskID, "sender_session_id": sessionID,
					"operation_id": operationID,
				}))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(resp.Payload, &payload))
			require.Equal(t, true, payload["claimed"])
			require.Equal(t, test.state, payload[sendOperationStateKey])
			require.Equal(t, test.retrySafe, payload[sendOperationRetryKey])
			require.NotContains(t, string(resp.Payload), "private prompt contents")
		})
	}
}

func TestGetTaskMessageOperationRejectsAnotherSession(t *testing.T) {
	_, repo := newTestTaskService(t)
	operation := &models.TaskMessageSendOperation{
		ID: "row-private", OperationID: "private-op", SenderTaskID: "sender-task",
		SenderSessionID: "owner-session", State: models.SendOperationStatePending,
	}
	require.NoError(t, repo.ClaimSendOperation(context.Background(), operation))
	h := &Handlers{logger: testLogger(t)}
	h.SetSendOperationStore(repo)

	resp, err := h.handleGetTaskMessageOperation(sendOperationTestContext("sender-task", "other-session"),
		makeWSMessage(t, ws.ActionMCPGetTaskMessageOperation, map[string]interface{}{
			"sender_task_id": "sender-task", "sender_session_id": "owner-session", "operation_id": "private-op",
		}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

var _ SendOperationStore = (*sqlite.Repository)(nil)
