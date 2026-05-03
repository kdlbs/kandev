package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleCreateWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		Name                        string  `json:"name"`
		Description                 string  `json:"description"`
		DefaultExecutorID           *string `json:"default_executor_id"`
		DefaultAgentProfileID       *string `json:"default_agent_profile_id"`
		DefaultConfigAgentProfileID *string `json:"default_config_agent_profile_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	workspace, err := h.taskSvc.CreateWorkspace(ctx, &service.CreateWorkspaceRequest{
		Name:                        req.Name,
		Description:                 req.Description,
		DefaultExecutorID:           req.DefaultExecutorID,
		DefaultAgentProfileID:       req.DefaultAgentProfileID,
		DefaultConfigAgentProfileID: req.DefaultConfigAgentProfileID,
	})
	if err != nil {
		h.logger.Error("failed to create workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workspace", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkspace(workspace))
}

func (h *Handlers) handleUpdateWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID                 string  `json:"workspace_id"`
		Name                        *string `json:"name"`
		Description                 *string `json:"description"`
		DefaultExecutorID           *string `json:"default_executor_id"`
		DefaultAgentProfileID       *string `json:"default_agent_profile_id"`
		DefaultConfigAgentProfileID *string `json:"default_config_agent_profile_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	workspace, err := h.taskSvc.UpdateWorkspace(ctx, req.WorkspaceID, &service.UpdateWorkspaceRequest{
		Name:                        req.Name,
		Description:                 req.Description,
		DefaultExecutorID:           req.DefaultExecutorID,
		DefaultAgentProfileID:       req.DefaultAgentProfileID,
		DefaultConfigAgentProfileID: req.DefaultConfigAgentProfileID,
	})
	if err != nil {
		h.logger.Error("failed to update workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workspace", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkspace(workspace))
}

func (h *Handlers) handleDeleteWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workspaceID, err := unmarshalStringField(msg.Payload, "workspace_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if workspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	ws2, _ := h.taskSvc.GetWorkspace(ctx, workspaceID)
	if err := h.taskSvc.DeleteWorkspace(ctx, workspaceID); err != nil {
		h.logger.Error("failed to delete workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete workspace", nil)
	}
	if ws2 != nil && h.eventBus != nil {
		_ = h.eventBus.Publish(ctx, events.WorkspaceDeleted, bus.NewEvent(
			events.WorkspaceDeleted,
			"mcp-handlers",
			map[string]interface{}{"id": ws2.ID, "name": ws2.Name},
		))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleGetWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workspaceID, err := unmarshalStringField(msg.Payload, "workspace_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if workspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	workspace, err := h.taskSvc.GetWorkspace(ctx, workspaceID)
	if err != nil {
		h.logger.Error("failed to get workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Workspace not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkspace(workspace))
}
