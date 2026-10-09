package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// proposeKindHandler backs coordinator.propose_resume, propose_message and
// propose_move. The coordinator id comes from the trusted principal, never
// the payload, and the actions exist only with features.coordinatorPhase2 on.
func (h *Handlers) proposeKindHandler(kind string) func(context.Context, *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		principal, ok := mcpscope.PrincipalFromContext(ctx)
		if !ok || !principal.IsCoordinator() {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
		}
		if h.coordinatorSvc == nil || !h.coordinatorSvc.Phase2() {
			response, _, err := coordinatorUnknownAction(msg)
			return response, err
		}
		var payload struct {
			StandingOrderIDs []string `json:"standing_order_ids"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		}
		proposal, deduplicated, err := h.coordinatorSvc.ProposeKind(ctx, principal.CoordinatorID, kind, msg.Payload, payload.StandingOrderIDs)
		if err != nil {
			return h.proposeKindErrorResponse(msg, kind, err)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"proposal_id":     proposal.ID,
			stopTaskStatusKey: string(proposal.Status),
			"deduplicated":    deduplicated,
		})
	}
}

func (h *Handlers) proposeKindErrorResponse(msg *ws.Message, kind string, err error) (*ws.Message, error) {
	var fieldErr *coordinator.FieldError
	if errors.As(err, &fieldErr) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldErr.Message,
			map[string]interface{}{fieldErrorDetailsKey: fieldErr.Field})
	}
	if errors.Is(err, coordinator.ErrCoordinatorProposalCapReached) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"this coordinator already has 25 open proposals", nil)
	}
	h.logger.Error("propose failed", zap.String("kind", kind), zap.Error(err))
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create proposal", nil)
}
