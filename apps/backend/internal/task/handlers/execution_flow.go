package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *MessageHandlers) wsAddMessageWithFlow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	ctx = context.WithValue(ctx, logger.RequestIDKey, msg.ID)
	var identity struct {
		TaskID          string `json:"task_id"`
		SessionID       string `json:"session_id"`
		ClientMessageID string `json:"client_message_id"`
		MessageID       string `json:"message_id"`
	}
	_ = msg.ParsePayload(&identity)
	if identity.ClientMessageID == "" {
		identity.ClientMessageID = identity.MessageID
	}
	finish := h.logger.BeginFlow(ctx, logger.ExecutionFlow{Stage: "request", TaskID: identity.TaskID, SessionID: identity.SessionID, MessageID: identity.ClientMessageID})
	response, err := h.wsAddMessage(ctx, msg)
	outcome, reason := "accepted", ""
	if err != nil || response == nil || response.Type == ws.MessageTypeError {
		outcome, reason = "rejected", "message_request_failed"
	}
	finish(outcome, reason)
	return response, err
}
