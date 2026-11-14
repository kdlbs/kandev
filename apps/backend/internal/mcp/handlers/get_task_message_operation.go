package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.uber.org/zap"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// getTaskMessageOperationRequest is the readback's wire shape. task_id is the
// caller's own task and is verified against the identity the MCP server
// injected; sender_task_id and sender_session_id are injected, never supplied
// by the model, exactly as message_task_kandev does.
type getTaskMessageOperationRequest struct {
	TaskID          string `json:"task_id"`
	OperationID     string `json:"operation_id"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

// handleGetTaskMessageOperation resolves one operation identity to its recorded
// outcome. It reads a single row and performs no delivery, so it is safe to call
// after a timeout, repeatedly, for as long as the record is retained.
//
// The row is looked up by the calling session's own (sender_task_id,
// sender_session_id) pair, so another session's identity is reported as not
// claimed and its existence never leaks. The answer is one of:
//
//   - state "pending": the first attempt may still commit. Wait and re-read; do
//     not resend.
//   - state "committed": it was delivered. Do not resend; the receipt names the
//     artifact.
//   - state "failed": only dispatch_rejected permits retry. outcome_unknown
//     still forbids resending.
//   - claimed=false: no retained record exists. Retention means this cannot
//     prove that no historical delivery occurred.
func (h *Handlers) handleGetTaskMessageOperation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req getTaskMessageOperationRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SenderTaskID == "" || req.SenderSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"sender_task_id and sender_session_id are required (the calling agent's MCP server must supply this)", nil)
	}
	if response := h.validateSendOperationSender(ctx, msg, req.SenderTaskID, req.SenderSessionID); response != nil {
		return response, nil
	}
	// The readback is scoped to the caller's own task. Accepting another task's
	// id would only ever miss, because the lookup is sender-scoped; saying so
	// explicitly keeps the tool from looking cross-task capable.
	if req.TaskID != "" && req.TaskID != req.SenderTaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"task_id must be your own task: an operation can only be read by the session that claimed it", nil)
	}
	if err := validateSendOperationIDParam(req.OperationID); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	if h.sendOperationStore == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"send-operation records are not available in this deployment", nil)
	}

	operation, err := h.sendOperationStore.GetSendOperation(
		ctx, req.SenderTaskID, req.SenderSessionID, req.OperationID)
	if err != nil {
		if errors.Is(err, models.ErrSendOperationNotFound) {
			return ws.NewResponse(msg.ID, msg.Action, unclaimedSendOperationPayload(req))
		}
		h.logger.Error("failed to read send operation",
			zap.String("operation_id", req.OperationID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to read the recorded send operation", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, claimedSendOperationPayload(operation))
}

// A caller cannot widen the server-injected sender pair through a raw action.
// Readback uses the existing kanban task principal; payload sender fields never
// establish authority.
func (h *Handlers) validateSendOperationSender(ctx context.Context, msg *ws.Message, taskID, sessionID string) *ws.Message {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.Surface != mcpprofile.SurfaceKanbanTask ||
		principal.CallerTaskID != taskID || principal.CallerSessionID != sessionID {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"send-operation records belong to the authorized calling task session")
	}
	return nil
}

// unclaimedSendOperationPayload reports only retained-record absence. It deliberately
// carries no delivery fields: there is no outcome to report, and a caller must
// not be tempted to read an absent field as "nothing was sent".
func unclaimedSendOperationPayload(req getTaskMessageOperationRequest) map[string]interface{} {
	return map[string]interface{}{
		"task_id":             req.SenderTaskID,
		sendOperationIDKey:    req.OperationID,
		sendOperationStateKey: "not_claimed",
		sendOperationRetryKey: false,
		"claimed":             false,
		"instruction":         "no retained record exists for this sender and operation_id; absence does not prove no historical delivery, so do not resend automatically",
	}
}

// claimedSendOperationPayload reports the recorded outcome and the one decision
// the caller has to make: whether resending is safe. It echoes no prompt and no
// other stored content.
func claimedSendOperationPayload(operation *models.TaskMessageSendOperation) map[string]interface{} {
	payload := map[string]interface{}{
		"task_id":             operation.SenderTaskID,
		sendOperationIDKey:    operation.OperationID,
		sendOperationStateKey: operation.State,
		sendOperationRetryKey: operation.RetrySafe(),
		"claimed":             true,
		"created_at":          operation.CreatedAt.UTC().Format(time.RFC3339),
	}
	if operation.TargetTaskID != "" {
		payload["target_task_id"] = operation.TargetTaskID
	}
	if operation.TargetSessionID != "" {
		payload["target_session_id"] = operation.TargetSessionID
	}
	if operation.DeliveryStatus != "" {
		payload[stopTaskStatusKey] = operation.DeliveryStatus
	}
	if operation.MessageID != "" {
		payload[sendOperationMessageKey] = operation.MessageID
	}
	if operation.QueuedEntryID != "" {
		payload[sendOperationQueuedKey] = operation.QueuedEntryID
	}
	if operation.FailureCode != "" {
		payload["failure_code"] = operation.FailureCode
	}
	if operation.SettledAt != nil {
		payload["settled_at"] = operation.SettledAt.UTC().Format(time.RFC3339)
	}
	switch operation.State {
	case models.SendOperationStatePending:
		payload["instruction"] = "the first attempt may still commit; do not resend, wait and read again"
	case models.SendOperationStateCommitted:
		payload["instruction"] = "delivered; do not resend"
	case models.SendOperationStateFailed:
		if operation.RetrySafe() {
			payload["instruction"] = "definitively not delivered; a fresh operation_id may be used"
		} else {
			payload["instruction"] = "delivery outcome is unknown; do not resend"
		}
	default:
		payload["instruction"] = "delivery outcome is unknown; do not resend"
	}
	return payload
}
