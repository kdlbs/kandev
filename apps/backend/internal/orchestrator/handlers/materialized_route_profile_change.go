package handlers

import (
	"errors"

	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func materializedRouteProfileChangeResponse(msg *ws.Message, err error) (*ws.Message, error) {
	if !errors.Is(err, orchestrator.ErrMaterializedRouteProfileChange) {
		return nil, nil
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
}
