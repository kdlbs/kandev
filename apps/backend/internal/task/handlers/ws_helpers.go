package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// wsIDOnlyRequest is the common request struct for WS handlers that take a single ID field.
type wsIDOnlyRequest struct {
	ID string `json:"id"`
}

// wsHandleIDRequest handles the common WS handler pattern for operations with a single "id" field.
// fn receives the ID and returns the response payload and any error.
func wsHandleIDRequest(
	ctx context.Context,
	msg *ws.Message,
	log *logger.Logger,
	errMsg string,
	fn func(ctx context.Context, id string) (any, error),
) (*ws.Message, error) {
	var req wsIDOnlyRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := fn(ctx, req.ID)
	if err != nil {
		log.Error(errMsg, zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, errMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
