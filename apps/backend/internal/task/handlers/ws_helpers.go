package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
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

// wsHandleIDRequestActiveCheck is like wsHandleIDRequest but also handles
// controller.ErrActiveTaskSessions with a specific validation error message.
func wsHandleIDRequestActiveCheck(
	ctx context.Context,
	msg *ws.Message,
	log *logger.Logger,
	errMsg, activeSessionMsg string,
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
		if errors.Is(err, controller.ErrActiveTaskSessions) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, activeSessionMsg, nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, errMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
