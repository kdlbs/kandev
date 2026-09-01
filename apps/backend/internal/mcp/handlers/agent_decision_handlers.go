package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// SetDashboardService wires the office dashboard service used to back the
// record_step_decision_kandev MCP tool. Optional — when nil, the handler
// reports a configuration error instead of registering and returning a
// broken endpoint.
func (h *Handlers) SetDashboardService(svc *dashboard.DashboardService) {
	h.dashboardSvc = svc
}

// handleRecordStepDecision dispatches mcp.record_step_decision
// (record_step_decision_kandev). Per the tool contract, task/step/role are
// never caller-supplied: the task comes from the calling session (msg
// carries session_id/task_id set by the MCP server from its own bound
// s.sessionID/s.taskID, matching every other Office tool), and role/seat
// are resolved server-side by DashboardService.RecordAgentDecision via the
// engine's AC-4a slate.
func (h *Handlers) handleRecordStepDecision(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.dashboardSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "dashboard service not configured", nil)
	}
	var req struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Decision  string `json:"decision"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and session_id are required", nil)
	}

	session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
		}
		h.logger.Error("record_step_decision: failed to load session",
			zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load session", nil)
	}
	if session.TaskID != req.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session does not belong to task", nil)
	}

	result, err := h.dashboardSvc.RecordAgentDecision(ctx, dashboard.RecordAgentDecisionInput{
		TaskID:         req.TaskID,
		AgentProfileID: session.AgentProfileID,
		Decision:       req.Decision,
		Reason:         req.Reason,
		SessionID:      req.SessionID,
	})
	if err != nil {
		if !errors.Is(err, shared.ErrForbidden) && !dashboard.IsAgentDecisionValidationError(err) {
			h.logger.Error("record_step_decision: failed to record decision", zap.String("task_id", req.TaskID), zap.Error(err))
		}
		return mapRecordStepDecisionError(msg, err)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"decision":           result.Decision,
		"role":               result.Role,
		"step_id":            result.StepID,
		"decision_id":        result.DecisionID,
		"decided_at":         result.DecidedAt,
		"transition_applied": result.TransitionApplied,
		"guards":             result.Guards,
	})
}

// mapRecordStepDecisionError classifies RecordAgentDecision failures into
// WS error codes. Only typed validation failures are safe to expose. Raw
// repository and engine failures become an opaque internal error.
func mapRecordStepDecisionError(msg *ws.Message, err error) (*ws.Message, error) {
	if errors.Is(err, shared.ErrForbidden) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	}
	if dashboard.IsAgentDecisionValidationError(err) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to record step decision", nil)
}
