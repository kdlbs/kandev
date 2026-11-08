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
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid JSON payload", nil)
		}
		proposal, deduplicated, err := h.coordinatorSvc.ProposeKind(ctx, principal.CoordinatorID, kind, msg.Payload, payload.StandingOrderIDs)
		if err != nil {
			return h.proposeKindErrorResponse(msg, kind, err)
		}
		result := map[string]interface{}{
			proposalIDKey:     proposal.ID,
			stopTaskStatusKey: string(proposal.Status),
			"deduplicated":    deduplicated,
		}
		if kind == coordinator.ProposalKindResume && !deduplicated {
			h.applyAutomaticApproval(ctx, proposal, result)
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	}
}

func (h *Handlers) proposeKindErrorResponse(msg *ws.Message, kind string, err error) (*ws.Message, error) {
	if errors.Is(err, coordinator.ErrNotFound) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "coordinator not found", nil)
	}
	var refusal *coordinator.ProposeRefusal
	if errors.As(err, &refusal) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, refusal.Code,
			map[string]interface{}{refusalCodeKey: refusal.Code, fieldErrorDetailsKey: refusal.Field})
	}
	var denied *coordinator.PolicyDeniedError
	if errors.As(err, &denied) {
		response, _, unknownErr := coordinatorUnknownAction(msg)
		return response, unknownErr
	}
	if errors.Is(err, coordinator.ErrAnswerTargetRead) {
		h.logger.Error("propose answer: target read failed", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "could not read the task's current question", nil)
	}
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

// handleProposeAnswer backs coordinator.propose_answer. The whole argument
// object belongs to the answer kind, which also owns the standing order
// citations, so the handler forwards the payload and no separate ids.
func (h *Handlers) handleProposeAnswer(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
	}
	if h.coordinatorSvc == nil || !h.coordinatorSvc.Phase2() {
		response, _, err := coordinatorUnknownAction(msg)
		return response, err
	}
	proposal, deduplicated, err := h.coordinatorSvc.ProposeKind(ctx, principal.CoordinatorID, coordinator.ProposalKindAnswer, msg.Payload, nil)
	if err != nil {
		return h.proposeKindErrorResponse(msg, coordinator.ProposalKindAnswer, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		proposalIDKey:     proposal.ID,
		stopTaskStatusKey: string(proposal.Status),
		"deduplicated":    deduplicated,
	})
}

// handleProposeImprovement backs coordinator.propose_improvement. The
// coordinator id comes from the trusted principal, never the payload, and the
// action exists only while phase 3 is effective.
func (h *Handlers) handleProposeImprovement(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
	}
	if h.coordinatorSvc == nil || !h.coordinatorSvc.Phase3Enabled() {
		response, _, err := coordinatorUnknownAction(msg)
		return response, err
	}
	proposal, err := h.coordinatorSvc.ProposeImprovement(ctx, principal.CoordinatorID, msg.Payload)
	if err != nil {
		if errors.Is(err, coordinator.ErrImprovementsUnavailable) {
			response, _, unknownErr := coordinatorUnknownAction(msg)
			return response, unknownErr
		}
		return h.proposeKindErrorResponse(msg, coordinator.ProposalKindImprovement, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		proposalIDKey:     proposal.ID,
		stopTaskStatusKey: string(proposal.Status),
	})
}

// proposalIDKey is the response key carrying a created proposal's id.
const proposalIDKey = "proposal_id"

// refusalCodeKey is the error-details key carrying a refusal code.
const refusalCodeKey = "code"

// handleProposeMerge backs coordinator.propose_merge. A refused creation is a
// normal answer with a null proposal, not an error.
func (h *Handlers) handleProposeMerge(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
	}
	if h.coordinatorSvc == nil {
		response, _, err := coordinatorUnknownAction(msg)
		return response, err
	}
	res, err := h.coordinatorSvc.ProposeMerge(ctx, principal.CoordinatorID, msg.Payload)
	if err != nil {
		return h.proposeKindErrorResponse(msg, coordinator.ProposalKindMerge, err)
	}
	out := map[string]interface{}{proposalIDKey: res.ProposalID, stopTaskStatusKey: res.Status, "deduplicated": res.Deduplicated}
	if res.Reason != "" {
		out["reason"] = res.Reason
	}
	if res.CreationCode != "" {
		out["creation_code"] = res.CreationCode
	}
	if res.OwningCoordinatorID != "" {
		out["owning_coordinator_id"] = res.OwningCoordinatorID
	}
	return ws.NewResponse(msg.ID, msg.Action, out)
}
