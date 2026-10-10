package handlers

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) wsRejectRetiredRecoveryBatch(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "batch interrupted session recovery is no longer supported", nil)
}
