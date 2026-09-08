package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type messageDeliveryRequest struct {
	DeliveryID      string `json:"delivery_id"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

// handleGetMessageDelivery returns a safe status projection to exactly the
// source or target session named by the receipt. Content and metadata stay in
// the durable ledger and are never exposed by this recovery API.
func (h *Handlers) handleGetMessageDelivery(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, failure := parseMessageDeliveryRequest(msg)
	if failure != nil {
		return failure, nil
	}
	delivery, failure := h.authorizeMessageDelivery(ctx, msg, req)
	if failure != nil {
		return failure, nil
	}
	return ws.NewResponse(msg.ID, msg.Action, messageDeliveryStatus(delivery))
}

// handleRetryMessageDelivery allows the same narrowly-authorized source or
// target session to reactivate a retained receipt. The worker, not this MCP
// call, performs later capacity admission.
func (h *Handlers) handleRetryMessageDelivery(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, failure := parseMessageDeliveryRequest(msg)
	if failure != nil {
		return failure, nil
	}
	_, failure = h.authorizeMessageDelivery(ctx, msg, req)
	if failure != nil {
		return failure, nil
	}
	queue := h.sessionLauncher.GetMessageQueue()
	delivery, err := queue.RetryDeliveryReceipt(ctx, req.DeliveryID)
	if err != nil {
		// ErrEntryNotFound here means RetryDelivery's compare-and-set found no
		// row in the recoverable state — a genuine client-facing conflict, not
		// a missing capability or storage failure. Everything else (an
		// unsupported ledger, a DB error) is an internal error: retrying is
		// not something the caller can fix by resubmitting the same request.
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "delivery is not recoverable", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to retry message delivery: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, messageDeliveryStatus(delivery))
}

func parseMessageDeliveryRequest(msg *ws.Message) (messageDeliveryRequest, *ws.Message) {
	var req messageDeliveryRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return req, messageDeliveryError(msg, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error())
	}
	req.DeliveryID = strings.TrimSpace(req.DeliveryID)
	req.SenderTaskID = strings.TrimSpace(req.SenderTaskID)
	req.SenderSessionID = strings.TrimSpace(req.SenderSessionID)
	if req.DeliveryID == "" || req.SenderTaskID == "" || req.SenderSessionID == "" {
		return req, messageDeliveryError(msg, ws.ErrorCodeValidation, "delivery_id and trusted sender task/session identity are required")
	}
	return req, nil
}

func (h *Handlers) authorizeMessageDelivery(ctx context.Context, msg *ws.Message, req messageDeliveryRequest) (*messagequeue.Delivery, *ws.Message) {
	if h.taskSvc == nil || h.sessionLauncher == nil || h.sessionLauncher.GetMessageQueue() == nil {
		return nil, messageDeliveryError(msg, ws.ErrorCodeInternalError, "message delivery recovery is not configured")
	}
	actor, err := h.taskSvc.GetTask(ctx, req.SenderTaskID)
	if err != nil || actor == nil {
		return nil, messageDeliveryError(msg, ws.ErrorCodeNotFound, "sender task not found")
	}
	actorSession, err := h.taskSvc.GetTaskSession(ctx, req.SenderSessionID)
	if err != nil || actorSession == nil || actorSession.TaskID != actor.ID {
		return nil, messageDeliveryError(msg, ws.ErrorCodeForbidden, "sender session does not belong to sender task")
	}
	delivery, err := h.sessionLauncher.GetMessageQueue().GetDeliveryReceipt(ctx, req.DeliveryID)
	if err != nil || delivery == nil {
		return nil, messageDeliveryError(msg, ws.ErrorCodeNotFound, "message delivery not found")
	}
	if (actor.ID != delivery.SenderTaskID || actorSession.ID != delivery.SenderSessionID) &&
		(actor.ID != delivery.TargetTaskID || actorSession.ID != delivery.TargetSessionID) {
		return nil, messageDeliveryError(msg, ws.ErrorCodeForbidden, "caller is not authorized to inspect this delivery")
	}
	return delivery, nil
}

func messageDeliveryStatus(delivery *messagequeue.Delivery) map[string]interface{} {
	return map[string]interface{}{
		"delivery_id": delivery.ID, "delivery_status": delivery.State,
		"sender_task_id": delivery.SenderTaskID, "sender_session_id": delivery.SenderSessionID,
		"target_task_id": delivery.TargetTaskID, "target_session_id": delivery.TargetSessionID,
		"queue_entry_id": delivery.QueueEntryID, "attempts": delivery.Attempts,
		"last_error": delivery.LastError, "created_at": delivery.CreatedAt,
		"updated_at": delivery.UpdatedAt, "delivered_at": delivery.DeliveredAt,
	}
}

func messageDeliveryError(msg *ws.Message, code, message string) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return response
}
