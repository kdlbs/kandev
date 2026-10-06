package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// fieldErrorDetailsKey is the details key naming the offending field in a
// ws.ErrorCodeValidation response for a *coordinator.FieldError.
const fieldErrorDetailsKey = "field"

// handleProposeTask backs coordinator.propose_task
// (docs/specs/coordinator/system-design/proposals.md#propose,
// AC-COORDINATOR-PROPOSALS-001.1 through .5). The coordinator id comes from
// the trusted principal, never the payload.
func (h *Handlers) handleProposeTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		Rationale    string `json:"rationale"`
		WorkflowID   string `json:"workflow_id"`
		StepID       string `json:"step_id"`
		RepositoryID string `json:"repository_id"`
		SourceTaskID string `json:"source_task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
	}
	if h.coordinatorSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "coordinator service is not available", nil)
	}

	req := coordinator.ProposeTaskRequest{
		Title:        payload.Title,
		Description:  payload.Description,
		Rationale:    payload.Rationale,
		WorkflowID:   payload.WorkflowID,
		StepID:       payload.StepID,
		RepositoryID: payload.RepositoryID,
		SourceTaskID: payload.SourceTaskID,
	}
	proposal, openCount, err := h.coordinatorSvc.ProposeTask(ctx, principal.CoordinatorID, req)
	if err != nil {
		return h.proposeTaskErrorResponse(msg, err)
	}

	if h.eventBus != nil {
		if pubErr := h.eventBus.Publish(ctx, events.CoordinatorUpdated, bus.NewEvent(
			events.CoordinatorUpdated, "mcp-handlers",
			coordinator.NewCoordinatorUpdatedPayload(proposal.WorkspaceID, proposal.CoordinatorID, openCount),
		)); pubErr != nil {
			h.logger.Warn("failed to publish coordinator.updated after propose_task",
				zap.String("coordinator_id", proposal.CoordinatorID), zap.Error(pubErr))
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"proposal_id":     proposal.ID,
		stopTaskStatusKey: string(proposal.Status),
	})
}

// proposeTaskErrorResponse maps a ProposeTask error to a WS error response:
// a *FieldError is a validation error naming its field
// (AC-COORDINATOR-PROPOSALS-001.3); the open-proposal cap is a validation
// error with no field (AC-COORDINATOR-PROPOSALS-001.4); anything else is
// logged and returned as an internal error.
func (h *Handlers) proposeTaskErrorResponse(msg *ws.Message, err error) (*ws.Message, error) {
	var fieldErr *coordinator.FieldError
	if errors.As(err, &fieldErr) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldErr.Message,
			map[string]interface{}{fieldErrorDetailsKey: fieldErr.Field})
	}
	if errors.Is(err, coordinator.ErrCoordinatorProposalCapReached) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"this coordinator already has 25 open proposals", nil)
	}
	h.logger.Error("propose_task failed", zap.Error(err))
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create proposal", nil)
}
