package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/ledger"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	turnsArgBefore = "before"
	turnsArgLimit  = "limit"
)

// CoordinatorTurnReader reads the coordinator's own turn ledger.
type CoordinatorTurnReader interface {
	List(ctx context.Context, coordinatorID string, args ledger.ListArgs) (*ledger.Page, error)
}

// SetCoordinatorTurnReader wires coordinator.list_turns. Leave it unset while
// the phase 3.1 surface is off so the action is unknown.
func (h *Handlers) SetCoordinatorTurnReader(r CoordinatorTurnReader) {
	h.coordinatorTurns = r
}

// handleListCoordinatorTurns backs coordinator.list_turns. The coordinator
// comes from the principal alone, so no payload field names another
// coordinator's rows. Read-only.
func (h *Handlers) handleListCoordinatorTurns(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, err := automationPayloadFields(msg.Payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "coordinator not found", nil)
	}
	if h.coordinatorTurns == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "turn ledger is not available", nil)
	}
	args, err := coordinatorTurnArgs(fields)
	if err != nil {
		return coordinatorTurnsError(msg, err)
	}
	page, err := h.coordinatorTurns.List(ctx, principal.CoordinatorID, args)
	switch {
	case err == nil:
		return ws.NewResponse(msg.ID, msg.Action, page)
	case errors.Is(err, ledger.ErrUnavailable):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "turn ledger is unavailable", nil)
	case errors.Is(err, coordinator.ErrNotFound):
		return coordinatorTurnsError(msg, err)
	default:
		var fe *coordinator.FieldError
		if errors.As(err, &fe) {
			return coordinatorTurnsError(msg, err)
		}
		h.logger.Warn("list_turns failed", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list turns", nil)
	}
}

func coordinatorTurnsError(msg *ws.Message, err error) (*ws.Message, error) {
	var fe *coordinator.FieldError
	if errors.As(err, &fe) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, fe.Message, map[string]interface{}{fieldErrorDetailsKey: fe.Field})
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
}

func coordinatorTurnArgs(fields map[string]json.RawMessage) (ledger.ListArgs, error) {
	var a ledger.ListArgs
	for name, dst := range map[string]*string{"since": &a.Since, "verdict": &a.Verdict, "trigger": &a.Trigger, "task": &a.Task, turnsArgBefore: &a.Before} {
		raw, present := fields[name]
		if !present || string(raw) == jsonNull {
			continue
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return a, &coordinator.FieldError{Field: name, Message: name + " must be a string"}
		}
	}
	if raw, present := fields[turnsArgLimit]; present && string(raw) != jsonNull {
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil || n != math.Trunc(n) {
			return a, &coordinator.FieldError{Field: turnsArgLimit, Message: "limit must be an integer"}
		}
		limit := int(n)
		a.Limit = &limit
	}
	return a, nil
}
