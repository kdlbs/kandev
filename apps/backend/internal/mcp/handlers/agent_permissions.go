package handlers

import (
	"context"
	"encoding/json"
	"errors"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type listPendingAgentPermissionsRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id,omitempty"`
}

type resolveAgentPermissionRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id"`
}

func (h *Handlers) handleListPendingAgentPermissions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request listPendingAgentPermissionsRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if request.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if h.agentPermissionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent permission service is not available", nil)
	}
	permissions, err := h.agentPermissionSvc.ListPendingAgentPermissions(ctx, request.TaskID, request.SessionID)
	if err != nil {
		return h.agentPermissionError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		keyTaskID:     request.TaskID,
		"permissions": permissions,
		keyTotal:      len(permissions),
	})
}

func (h *Handlers) handleResolveAgentPermission(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request resolveAgentPermissionRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	for _, required := range []struct {
		field string
		value string
	}{
		{field: keyTaskID, value: request.TaskID},
		{field: keySessionID, value: request.SessionID},
		{field: "request_id", value: request.RequestID},
		{field: "pending_id", value: request.PendingID},
		{field: "option_id", value: request.OptionID},
	} {
		if required.value == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, required.field+" is required", nil)
		}
	}
	if !mcporigin.IsTrustedExternalTransport(ctx) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Agent permission resolution requires external MCP", nil)
	}
	if h.agentPermissionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent permission service is not available", nil)
	}
	result, err := h.agentPermissionSvc.ResolveAgentPermission(ctx, orchestrator.ResolveAgentPermissionRequest{
		TaskID: request.TaskID, SessionID: request.SessionID, RequestID: request.RequestID,
		PendingID: request.PendingID, OptionID: request.OptionID, Source: models.PermissionSourceExternalMCP,
	})
	if err != nil {
		return h.agentPermissionError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) agentPermissionError(msg *ws.Message, err error) (*ws.Message, error) {
	for _, domainErr := range []error{
		orchestrator.ErrPermissionTaskOrSessionNotFound,
		orchestrator.ErrPermissionNotFound,
		orchestrator.ErrPermissionStale,
		orchestrator.ErrPermissionAlreadyResolved,
		orchestrator.ErrPermissionResolutionInProgress,
		orchestrator.ErrPermissionOptionNotOffered,
		orchestrator.ErrPermissionAuditFailed,
		orchestrator.ErrPermissionDeliveryFailed,
	} {
		if errors.Is(err, domainErr) {
			return ws.NewError(msg.ID, msg.Action, domainErr.Error(), permissionErrorMessage(domainErr), nil)
		}
	}
	h.logger.Error("agent permission operation failed")
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Agent permission operation failed", nil)
}

func permissionErrorMessage(err error) string {
	switch err {
	case orchestrator.ErrPermissionTaskOrSessionNotFound:
		return "Task or session was not found"
	case orchestrator.ErrPermissionNotFound:
		return "Pending permission request was not found"
	case orchestrator.ErrPermissionStale:
		return "Pending permission request is stale or was replaced"
	case orchestrator.ErrPermissionAlreadyResolved:
		return "Permission request was already resolved"
	case orchestrator.ErrPermissionResolutionInProgress:
		return "Permission request is being resolved by another caller"
	case orchestrator.ErrPermissionOptionNotOffered:
		return "Selected option was not offered for this permission request"
	case orchestrator.ErrPermissionAuditFailed:
		return "Permission resolution could not be recorded"
	case orchestrator.ErrPermissionDeliveryFailed:
		return "Permission resolution could not be delivered to the agent"
	default:
		return "Agent permission operation failed"
	}
}
