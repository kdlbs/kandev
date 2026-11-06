package handlers

import (
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func isQueueIdentityError(err error) bool {
	return errors.Is(err, messagequeue.ErrSessionIdentityMismatch) ||
		errors.Is(err, messagequeue.ErrTaskInactive)
}

func queueAdmissionErrorResponse(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable,
		"Session queue is busy; retry the operation", nil)
}
