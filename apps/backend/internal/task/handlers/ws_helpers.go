package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// wsIDOnlyRequest is the common request struct for WS handlers that take a single ID field.
type wsIDOnlyRequest struct {
	ID string `json:"id"`
}

// wsDeleteWithActiveSessionCheck handles WS delete requests for resources that block on active sessions.
// It parses the request ID, calls deleteFn, and returns a validation error if the resource is in use.
func wsDeleteWithActiveSessionCheck(
	ctx context.Context,
	msg *ws.Message,
	log *logger.Logger,
	resourceName string,
	deleteFn func(ctx context.Context, id string) error,
) (*ws.Message, error) {
	var req wsIDOnlyRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if err := deleteFn(ctx, req.ID); err != nil {
		log.Error("failed to delete "+resourceName, zap.Error(err))
		if errors.Is(err, service.ErrActiveTaskSessions) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, resourceName+" is used by an active agent session", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to delete "+resourceName, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
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
