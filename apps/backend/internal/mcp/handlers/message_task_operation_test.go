package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type memorySendOperationStore struct {
	rows   map[string]*models.TaskMessageSendOperation
	claims int
}

func newMemorySendOperationStore() *memorySendOperationStore {
	return &memorySendOperationStore{rows: map[string]*models.TaskMessageSendOperation{}}
}

func (s *memorySendOperationStore) ClaimSendOperation(_ context.Context, operation *models.TaskMessageSendOperation) error {
	key := operation.SenderTaskID + "|" + operation.SenderSessionID + "|" + operation.OperationID
	if _, ok := s.rows[key]; ok {
		return models.ErrSendOperationNotClaimed
	}
	s.claims++
	operation.State = models.SendOperationStatePending
	stored := *operation
	s.rows[key] = &stored
	return nil
}

func (s *memorySendOperationStore) GetSendOperation(_ context.Context, senderTaskID, senderSessionID, operationID string) (*models.TaskMessageSendOperation, error) {
	row, ok := s.rows[senderTaskID+"|"+senderSessionID+"|"+operationID]
	if !ok {
		return nil, models.ErrSendOperationNotFound
	}
	return row, nil
}

func (s *memorySendOperationStore) SettleSendOperation(_ context.Context, senderTaskID, senderSessionID, operationID, state, targetSessionID, deliveryStatus, messageID, queuedEntryID, failureCode string) (bool, error) {
	row, ok := s.rows[senderTaskID+"|"+senderSessionID+"|"+operationID]
	if !ok || row.State != models.SendOperationStatePending {
		return false, nil
	}
	row.State, row.TargetSessionID, row.DeliveryStatus = state, targetSessionID, deliveryStatus
	row.MessageID, row.QueuedEntryID, row.FailureCode = messageID, queuedEntryID, failureCode
	return true, nil
}

func kanbanSenderContext(senderTaskID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-1", Surface: mcpprofile.SurfaceKanbanTask,
		CallerTaskID: senderTaskID, CallerSessionID: "sender-sess-1",
	})
}

func decodeResponsePayload(t *testing.T, resp *ws.Message) map[string]interface{} {
	t.Helper()
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	return payload
}

func TestHandleMessageTask_ReportRejectsOperationID(t *testing.T) {
	h := &Handlers{}
	payload := senderPayloadWithMode("task-2", "status", "task-1", "report")
	payload["session_id"] = "sess-2"
	payload[sendOperationIDParam] = "op-1"

	resp, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, payload))

	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

// A replay-only send answers from the sender's retained record without
// resolving the target, and never claims or dispatches when none exists.
func TestHandleMessageTask_ReplayOnlyReadsReceiptBeforeTarget(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, _, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)
	store := newMemorySendOperationStore()
	h.SetSendOperationStore(store)
	require.NoError(t, store.ClaimSendOperation(context.Background(), &models.TaskMessageSendOperation{
		OperationID: "op-1", SenderTaskID: sender.ID, SenderSessionID: "sender-sess-1",
		TargetTaskID: "deleted-task", RequestedPrompt: "hello",
	}))
	ctx := WithSendOperationReplayOnly(kanbanSenderContext(sender.ID))

	replay := senderPayload("deleted-task", "hello", sender.ID)
	replay[sendOperationIDParam] = "op-1"
	resp, err := h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, replay))
	require.NoError(t, err)
	receipt := decodeResponsePayload(t, resp)
	require.Equal(t, true, receipt["replayed"])
	require.Equal(t, models.SendOperationStatePending, receipt[sendOperationStateKey])
	require.Equal(t, false, receipt[sendOperationRetryKey])
	require.Contains(t, receipt["instruction"], "do not resend")

	fresh := senderPayload("deleted-task", "hello", sender.ID)
	fresh[sendOperationIDParam] = "op-2"
	resp, err = h.handleMessageTask(ctx, makeWSMessage(t, ws.ActionMCPMessageTask, fresh))
	require.NoError(t, err)
	assertWSError(t, resp, SendOperationNotRetainedCode)
	require.Equal(t, 1, store.claims, "a replay-only call must not claim a new identity")
	require.Empty(t, orch.startCreatedCalls)
}

func TestHandleMessageTask_AlreadyAnsweredEchoesOperationID(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, _, childSession := seedParentQuestionScenario(t, svc, repo)
	h, _ := newMessageTaskHandler(t, svc, repo)
	h.inputPauser = &recordingClarificationInputPauser{}
	h.SetSendOperationStore(newMemorySendOperationStore())

	questionResp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, child.ID, childSession.ID))
	require.NoError(t, err)
	questionID := decodeResponsePayload(t, questionResp)["question_id"].(string)
	answer := senderPayload(child.ID, "Use Postgres.", parent.ID)
	answer["reply_to_question_id"] = questionID
	_, err = h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, answer))
	require.NoError(t, err)

	answer[sendOperationIDParam] = "answer-again-1"
	resp, err := h.handleMessageTask(kanbanSenderContext(parent.ID), makeWSMessage(t, ws.ActionMCPMessageTask, answer))

	require.NoError(t, err)
	payload := decodeResponsePayload(t, resp)
	require.Equal(t, "already_answered", payload["status"])
	require.Equal(t, "answer-again-1", payload[sendOperationIDKey])
}
