package handlers

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type wsRetryChildStallRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
}

// wsRetryChildStall makes a failed child stall alert eligible for delivery
// again under its original identity.
func (h *Handlers) wsRetryChildStall(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRetryChildStallRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" || req.TurnID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id and turn_id are required", nil)
	}
	retried, err := h.service.RetryChildStall(ctx, req.TaskID, req.SessionID, req.TurnID)
	if err != nil {
		h.logger.Warn("failed to retry child stall alert", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to retry child stall alert: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"retried": retried, "turn_id": req.TurnID})
}
