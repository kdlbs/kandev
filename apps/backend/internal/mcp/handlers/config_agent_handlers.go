package handlers

import (
	"context"
	"encoding/json"
	"errors"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleListAgents(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.agentSettingsCtrl.ListAgents(ctx)
	if err != nil {
		h.logger.Error("failed to list agents", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list agents", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleUpdateAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		AgentID       string  `json:"agent_id"`
		SupportsMCP   *bool   `json:"supports_mcp"`
		MCPConfigPath *string `json:"mcp_config_path"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	agent, err := h.agentSettingsCtrl.UpdateAgent(ctx, agentsettingscontroller.UpdateAgentRequest{
		ID:            req.AgentID,
		SupportsMCP:   req.SupportsMCP,
		MCPConfigPath: req.MCPConfigPath,
	})
	if err != nil {
		h.logger.Error("failed to update agent", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update agent", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, agent)
}

func (h *Handlers) handleListAgentProfiles(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	agentID, err := unmarshalStringField(msg.Payload, "agent_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if agentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}

	agent, err := h.agentSettingsCtrl.GetAgent(ctx, agentID)
	if err != nil {
		h.logger.Error("failed to get agent profiles", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get agent profiles", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"profiles": agent.Profiles,
		"total":    len(agent.Profiles),
	})
}

func (h *Handlers) handleCreateAgentProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		AgentID     string `json:"agent_id"`
		Name        string `json:"name"`
		Model       string `json:"model"`
		AutoApprove bool   `json:"auto_approve"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}
	if req.Model == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "model is required", nil)
	}

	profile, err := h.agentSettingsCtrl.CreateProfile(ctx, agentsettingscontroller.CreateProfileRequest{
		AgentID: req.AgentID,
		Name:    req.Name,
		Model:   req.Model,
	})
	if err != nil {
		h.logger.Error("failed to create agent profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create agent profile: "+err.Error(), nil)
	}
	h.publishAgentProfileEvent(ctx, events.AgentProfileCreated, profile)
	return ws.NewResponse(msg.ID, msg.Action, profile)
}

func (h *Handlers) handleDeleteAgentProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	profileID, err := unmarshalStringField(msg.Payload, "profile_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if profileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}

	profile, err := h.agentSettingsCtrl.DeleteProfile(ctx, profileID, false)
	if err != nil {
		h.logger.Error("failed to delete agent profile", zap.Error(err))
		var inUseErr *agentsettingscontroller.ErrProfileInUseDetail
		if errors.As(err, &inUseErr) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Cannot delete: profile is used by an active agent session", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete agent profile: "+err.Error(), nil)
	}
	h.publishAgentProfileEvent(ctx, events.AgentProfileDeleted, profile)
	return ws.NewResponse(msg.ID, msg.Action, profile)
}

func (h *Handlers) handleUpdateAgentProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ProfileID string  `json:"profile_id"`
		Name      *string `json:"name"`
		Model     *string `json:"model"`
		Mode      *string `json:"mode"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}

	profile, err := h.agentSettingsCtrl.UpdateProfile(ctx, agentsettingscontroller.UpdateProfileRequest{
		ID:    req.ProfileID,
		Name:  req.Name,
		Model: req.Model,
		Mode:  req.Mode,
	})
	if err != nil {
		h.logger.Error("failed to update agent profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update agent profile", nil)
	}
	h.publishAgentProfileEvent(ctx, events.AgentProfileUpdated, profile)
	return ws.NewResponse(msg.ID, msg.Action, profile)
}

// publishAgentProfileEvent publishes an agent profile event to the event bus.
// The payload wraps the profile in a "profile" key to match the format expected
// by existing frontend WS handlers (same format as HTTP agent settings handlers).
func (h *Handlers) publishAgentProfileEvent(ctx context.Context, eventType string, profile interface{}) {
	if h.eventBus == nil || profile == nil {
		return
	}
	data := map[string]interface{}{
		"profile": profile,
	}
	if err := h.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "mcp-handlers", data)); err != nil {
		h.logger.Error("failed to publish agent profile event",
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}
