package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	PendingMoveReadFailedCode    = "pending_move_read_failed"
	pendingMoveReadFailedMessage = "Pending move read failed."
)

type readPendingMoveRequest struct {
	TaskID string `json:"task_id"`
}

// handleReadPendingMove exposes the read-only census companion to
// handleCancelPendingMove. It never mutates pending-move state: an authorized
// request with no armed row returns found=false, a proven zero-row result,
// not an error.
func (h *Handlers) handleReadPendingMove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	actor := pendingMoveCancellationActor(ctx)
	if h.pendingMoveReader == nil {
		return ws.NewError(msg.ID, msg.Action, PendingMoveReadFailedCode, pendingMoveReadFailedMessage, nil)
	}
	var req readPendingMoveRequest
	decoder := json.NewDecoder(bytes.NewReader(msg.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		_, auditErr := h.pendingMoveReader.ReadPendingMove(ctx, actor, "", msg.ID)
		if auditErr != nil {
			return pendingMoveReadError(msg, auditErr)
		}
		return ws.NewError(msg.ID, msg.Action, PendingMoveInvalidArgumentCode, pendingMoveInvalidArgumentMessage, nil)
	}
	result, err := h.pendingMoveReader.ReadPendingMove(ctx, actor, req.TaskID, msg.ID)
	if err != nil {
		return pendingMoveReadError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func pendingMoveReadError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, messagequeue.ErrPendingMoveInvalidArgument):
		return ws.NewError(msg.ID, msg.Action, PendingMoveInvalidArgumentCode, pendingMoveInvalidArgumentMessage, nil)
	case errors.Is(err, messagequeue.ErrPendingMoveNotFoundOrChanged):
		return ws.NewError(msg.ID, msg.Action, PendingMoveNotFoundOrChangedCode, pendingMoveNotFoundMessage, nil)
	default:
		return ws.NewError(msg.ID, msg.Action, PendingMoveReadFailedCode, pendingMoveReadFailedMessage, nil)
	}
}
