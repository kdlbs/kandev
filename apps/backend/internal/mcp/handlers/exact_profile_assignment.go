package handlers

import (
	"context"
	"encoding/json"
	"math"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) handleAssignExactTaskProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		AgentProfileID string `json:"agent_profile_id"`
		Generation     int64  `json:"generation"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.AgentProfileID == "" || req.Generation < 1 || req.Generation == math.MinInt64 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, agent_profile_id, and positive generation are required", nil)
	}
	if !mcporigin.IsTrustedExternalTransport(ctx) {
		principal, ok := mcpscope.PrincipalFromContext(ctx)
		if !ok || req.TaskID != principal.CallerTaskID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task_id does not match session task", nil)
		}
	}
	if h.exactTaskProfileAssigner == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "exact task profile assignment is unavailable", nil)
	}
	result, err := h.exactTaskProfileAssigner.AssignExactTaskProfile(ctx, req.TaskID, req.AgentProfileID, req.Generation)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}
