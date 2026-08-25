package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// notFoundMessage is the single reply for anything the caller may not see and
// anything that does not exist: one wording, so the frame itself cannot be
// used to tell the two apart.
const notFoundMessage = "Not found"

func (h *Handlers) wsGetByID(
	ctx context.Context, msg *ws.Message,
	logErrMsg, clientErrMsg string,
	fn func(context.Context, string) (any, error),
) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := fn(ctx, req.ID)
	if err != nil {
		h.logger.Error(logErrMsg, zap.Error(err))
		// A denial and a genuine miss share one reply. Anything else is a
		// failure and must not be dressed up as a missing resource — the
		// authorization step reads the repository, so a database error can
		// reach here now.
		if service.IsNotFound(err) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, clientErrMsg, nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsHandleStringField(
	ctx context.Context, msg *ws.Message,
	fieldValue, fieldName, logErrMsg, clientErrMsg string,
	fn func(context.Context, string) (any, error),
) (*ws.Message, error) {
	if fieldValue == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldName+" is required", nil)
	}
	resp, err := fn(ctx, fieldValue)
	if err != nil {
		h.logger.Error(logErrMsg, zap.Error(err))
		if errors.Is(err, service.ErrNotVisible) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, notFoundMessage, nil)
		}

		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
