package handlers

import (
	"context"
	"encoding/json"
	"errors"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
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
	var envelope struct {
		AgentID     string                     `json:"agent_id"`
		Name        string                     `json:"name"`
		Model       *string                    `json:"model"`
		AutoApprove *bool                      `json:"auto_approve"`
		Settings    map[string]json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(msg.Payload, &envelope); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if envelope.AgentID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_id is required", nil)
	}
	if envelope.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}
	for key := range envelope.Settings {
		if key == "agent_id" || key == "name" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "settings cannot override agent_id or name", map[string]interface{}{"field": key})
		}
	}
	settings := make(map[string]json.RawMessage, len(envelope.Settings)+3)
	for key, value := range envelope.Settings {
		settings[key] = value
	}
	if envelope.Model != nil {
		if _, exists := settings["model"]; exists {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "model was supplied both at the top level and in settings", nil)
		}
		encoded, _ := json.Marshal(*envelope.Model)
		settings["model"] = encoded
	}
	if envelope.AutoApprove != nil {
		if _, exists := settings["auto_approve"]; exists {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "auto_approve was supplied both at the top level and in settings", nil)
		}
		encoded, _ := json.Marshal(*envelope.AutoApprove)
		settings["auto_approve"] = encoded
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid settings payload", nil)
	}
	var req agentsettingsdto.ProfileCreateRequest
	if err := json.Unmarshal(encoded, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid settings payload: "+err.Error(), nil)
	}
	req.AgentID = envelope.AgentID
	req.Name = envelope.Name
	if err := req.Validate(); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}

	profile, err := h.agentSettingsCtrl.CreateProfile(ctx, agentsettingscontroller.CreateProfileRequestFromDTO(req))
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
		var inUseErr *agentsettingscontroller.ErrProfileInUseDetail
		if errors.As(err, &inUseErr) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Cannot delete: profile is used by an active agent session", nil)
		}
		if errors.Is(err, agentsettingscontroller.ErrAgentProfileNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Agent profile not found", nil)
		}
		// Only an unclassified failure is a server error. Deleting a profile
		// that is gone, or one still in use, are client conditions, and the
		// REST sibling likewise logs only in its 500 branch.
		h.logger.Error("failed to delete agent profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete agent profile: "+err.Error(), nil)
	}
	h.publishAgentProfileEvent(ctx, events.AgentProfileDeleted, profile)
	return ws.NewResponse(msg.ID, msg.Action, profile)
}

func (h *Handlers) handleUpdateAgentProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var envelope struct {
		ProfileID string                     `json:"profile_id"`
		Settings  map[string]json.RawMessage `json:"settings"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(msg.Payload, &raw); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if err := json.Unmarshal(msg.Payload, &envelope); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if envelope.ProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}
	settings := make(map[string]json.RawMessage, len(raw)+len(envelope.Settings))
	for key, value := range raw {
		if key == "profile_id" || key == "settings" {
			continue
		}
		settings[key] = value
	}
	for key, value := range envelope.Settings {
		if _, exists := settings[key]; exists {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "setting was supplied both at the top level and in settings", map[string]interface{}{"field": key})
		}
		settings[key] = value
	}
	encodedID, _ := json.Marshal(envelope.ProfileID)
	settings["id"] = encodedID
	encoded, err := json.Marshal(settings)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid profile settings", nil)
	}
	var req agentsettingsdto.ProfileUpdateRequest
	if err := json.Unmarshal(encoded, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid profile settings: "+err.Error(), nil)
	}
	if err := req.Validate(); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	profile, err := h.agentSettingsCtrl.UpdateProfile(ctx, agentsettingscontroller.UpdateProfileRequestFromDTO(req))
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
