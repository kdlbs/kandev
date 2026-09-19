package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type coordinatorHandoffRequest struct {
	TaskID                        string `json:"task_id"`
	PredecessorSessionID          string `json:"predecessor_session_id"`
	SuccessorSessionID            string `json:"successor_session_id"`
	PredecessorQueueIncarnationID string `json:"predecessor_queue_incarnation_id"`
	SuccessorQueueIncarnationID   string `json:"successor_queue_incarnation_id"`
	ExpectedAgentProfileID        string `json:"expected_agent_profile_id"`
	ExpectedModel                 string `json:"expected_model"`
	OperationID                   string `json:"operation_id"`
}

func (h *Handlers) handleHandoffCoordinatorPrimary(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req coordinatorHandoffRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !h.isCanonicalCoordinator(ctx, principal) || principal.CallerTaskID != req.TaskID ||
		principal.CallerSessionID != req.PredecessorSessionID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"coordinator handoff requires the current canonical Coordinator session", nil)
	}
	if h.coordinatorHandoffSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "coordinator handoff is unavailable", nil)
	}
	result, err := h.coordinatorHandoffSvc.HandoffCoordinatorPrimary(ctx, orchestrator.CoordinatorHandoffRequest{
		TaskID: strings.TrimSpace(req.TaskID), PredecessorSessionID: strings.TrimSpace(req.PredecessorSessionID),
		SuccessorSessionID:            strings.TrimSpace(req.SuccessorSessionID),
		PredecessorQueueIncarnationID: strings.TrimSpace(req.PredecessorQueueIncarnationID),
		SuccessorQueueIncarnationID:   strings.TrimSpace(req.SuccessorQueueIncarnationID),
		ExpectedAgentProfileID:        strings.TrimSpace(req.ExpectedAgentProfileID),
		ExpectedModel:                 strings.TrimSpace(req.ExpectedModel), OperationID: strings.TrimSpace(req.OperationID),
	})
	if err != nil {
		if errors.Is(err, orchestrator.ErrCoordinatorHandoffConflict) ||
			errors.Is(err, orchestrator.ErrCoordinatorSuccessorModel) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}
