package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleGetMcpConfig(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	profileID, err := unmarshalStringField(msg.Payload, "profile_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if profileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}

	config, err := h.mcpConfigSvc.GetConfigByProfileID(ctx, profileID)
	if err != nil {
		h.logger.Error("failed to get MCP config", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get MCP config", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, config)
}

func (h *Handlers) handleUpdateMcpConfig(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ProfileID string                         `json:"profile_id"`
		Enabled   *bool                          `json:"enabled"`
		Servers   map[string]mcpconfig.ServerDef `json:"servers"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}

	updated, err := h.mcpConfigSvc.PatchConfigByProfileID(ctx, req.ProfileID, mcpconfig.ConfigPatch{
		Enabled: req.Enabled,
		Servers: serverDefsPointer(req.Servers),
	})
	if err != nil {
		h.logger.Error("failed to update MCP config", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update MCP config", nil)
	}
	h.broadcastProfileMCPConfigUpdated(req.ProfileID, updated.WorkspaceID)
	return ws.NewResponse(msg.ID, msg.Action, updated)
}

func serverDefsPointer(value map[string]mcpconfig.ServerDef) *map[string]mcpconfig.ServerDef {
	if value == nil {
		return nil
	}
	return &value
}

func (h *Handlers) broadcastProfileMCPConfigUpdated(profileID, workspaceID string) {
	if h.settingsBroadcaster == nil || profileID == "" {
		return
	}
	var scopedWorkspaceID any
	if workspaceID != "" {
		scopedWorkspaceID = workspaceID
	}
	notification, err := ws.NewNotification(ws.ActionAgentProfileMCPConfigUpdated, map[string]any{
		"profile_id":   profileID,
		"workspace_id": scopedWorkspaceID,
	})
	if err != nil {
		return
	}
	if workspaceID != "" {
		workspaceHub, ok := h.settingsBroadcaster.(interface {
			BroadcastToWorkspaceOrDrop(string, *ws.Message)
		})
		if ok {
			workspaceHub.BroadcastToWorkspaceOrDrop(workspaceID, notification)
		}
		return
	}
	//ws:global profile MCP updates without workspace ownership are global.
	h.settingsBroadcaster.Broadcast(notification)
}
