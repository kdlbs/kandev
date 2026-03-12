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

	// Get existing config to merge with
	existing, err := h.mcpConfigSvc.GetConfigByProfileID(ctx, req.ProfileID)
	if err != nil {
		h.logger.Error("failed to get existing MCP config", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get existing MCP config", nil)
	}

	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Servers != nil {
		existing.Servers = req.Servers
	}

	updated, err := h.mcpConfigSvc.UpsertConfigByProfileID(ctx, req.ProfileID, existing)
	if err != nil {
		h.logger.Error("failed to update MCP config", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update MCP config", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, updated)
}
