package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type updateProfileMcpConfigRequest struct {
	Enabled    *bool                          `json:"enabled"`
	Servers    map[string]mcpconfig.ServerDef `json:"servers"`
	MCPServers map[string]mcpconfig.ServerDef `json:"mcpServers"`
	Meta       map[string]any                 `json:"meta,omitempty"`
}

func (h *Handlers) httpGetProfileMcpConfig(c *gin.Context) {
	profileID := c.Param("id")
	if profileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile id is required"})
		return
	}

	resp, err := h.controller.GetAgentProfileMcpConfig(c.Request.Context(), profileID)
	if err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		if err == controller.ErrAgentMcpUnsupported {
			c.JSON(http.StatusBadRequest, gin.H{"error": "mcp not supported by agent"})
			return
		}
		h.logger.Error("failed to get mcp config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get mcp config"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpUpdateProfileMcpConfig(c *gin.Context) {
	profileID := c.Param("id")
	if profileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile id is required"})
		return
	}

	var body updateProfileMcpConfigRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
		return
	}
	servers := body.Servers
	if len(servers) == 0 && len(body.MCPServers) > 0 {
		servers = body.MCPServers
	}
	if servers == nil {
		servers = map[string]mcpconfig.ServerDef{}
	}

	resp, err := h.controller.UpdateAgentProfileMcpConfig(c.Request.Context(), profileID, controller.UpdateAgentProfileMcpConfigRequest{
		Enabled: *body.Enabled,
		Servers: servers,
		Meta:    body.Meta,
	})
	if err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		if err == controller.ErrAgentMcpUnsupported {
			c.JSON(http.StatusBadRequest, gin.H{"error": "mcp not supported by agent"})
			return
		}
		h.logger.Error("failed to update mcp config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update mcp config"})
		return
	}
	c.JSON(http.StatusOK, resp)
	h.broadcastProfileMCPConfigUpdated(profileID, resp.WorkspaceID)
}

func profileValidationMessage(err error, requiredValue, missingMessage string) string {
	if strings.TrimSpace(requiredValue) == "" {
		return missingMessage
	}
	return err.Error()
}

type createProfileRequest = dto.ProfileCreateRequest

func (h *Handlers) httpCreateProfile(c *gin.Context) {
	var body createProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	body.AgentID = c.Param("id")
	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": profileValidationMessage(err, body.Name, "profile name is required")})
		return
	}
	resp, err := h.controller.CreateProfile(c.Request.Context(), controller.CreateProfileRequestFromDTO(body))
	if err != nil {
		if errors.Is(err, controller.ErrDynamicAgentRoutingDisabled) || errors.Is(err, controller.ErrAgentFeatureDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, controller.ErrDynamicProfileCandidatesRequired) ||
			errors.Is(err, controller.ErrDynamicProfilePositions) ||
			errors.Is(err, controller.ErrDynamicProfileRule) ||
			errors.Is(err, controller.ErrDynamicProfileCandidate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, controller.ErrInvalidProfileEnvVars) || errors.Is(err, controller.ErrInvalidCommandPrefix) ||
			errors.Is(err, controller.ErrInvalidProviderConfig) ||
			errors.Is(err, controller.ErrRequireExactModelNeedsModel) || errors.Is(err, controller.ErrRequireExactModelUnsupported) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create profile"})
		return
	}
	h.broadcastProfileEvent(c.Request.Context(), ws.ActionAgentProfileCreated, resp)
	c.JSON(http.StatusOK, resp)
}

type updateProfileRequest = dto.ProfileUpdateRequest

func (h *Handlers) httpUpdateProfile(c *gin.Context) {
	var body updateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	body.ID = c.Param("id")
	body.Force = c.Query("force") == queryTrue
	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": profileValidationMessage(err, body.ID, "profile id is required")})
		return
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile name is required"})
		return
	}
	resp, err := h.controller.UpdateProfile(c.Request.Context(), controller.UpdateProfileRequestFromDTO(body))
	if err != nil {
		if writeProfileUpdateError(c, err) {
			return
		}
		h.logger.Error("failed to update profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	h.broadcastProfileEvent(c.Request.Context(), ws.ActionAgentProfileUpdated, resp)
	c.JSON(http.StatusOK, resp)
}

func writeProfileUpdateError(c *gin.Context, err error) bool {
	switch {
	case err == controller.ErrAgentProfileNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
	case isInvalidProfileUpdateError(err), isInvalidDynamicProfileUpdateError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case isProfileUpdateConflictError(err):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		var inUseErr *controller.ErrProfileInUseDetail
		if !errors.As(err, &inUseErr) {
			return false
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":            "agent profile is in use",
			"utility_agents":   inUseErr.UtilityAgents,
			"dynamic_profiles": inUseErr.DynamicProfiles,
		})
	}
	return true
}

func isInvalidProfileUpdateError(err error) bool {
	return errors.Is(err, controller.ErrInvalidProfileEnvVars) ||
		errors.Is(err, controller.ErrInvalidCommandPrefix) ||
		errors.Is(err, controller.ErrInvalidProviderConfig) ||
		errors.Is(err, controller.ErrRequireExactModelNeedsModel) ||
		errors.Is(err, controller.ErrRequireExactModelUnsupported)
}

func isInvalidDynamicProfileUpdateError(err error) bool {
	return errors.Is(err, controller.ErrDynamicProfileCandidatesRequired) ||
		errors.Is(err, controller.ErrDynamicProfilePositions) ||
		errors.Is(err, controller.ErrDynamicProfileRule) ||
		errors.Is(err, controller.ErrDynamicProfileCandidate)
}

func isProfileUpdateConflictError(err error) bool {
	return errors.Is(err, controller.ErrDynamicAgentRoutingDisabled) ||
		errors.Is(err, controller.ErrDynamicProfileVersionConflict)
}

// httpDuplicateProfile copies a profile's full configuration into a new row
// named "<source> copy" and returns the new profile. No request body: the
// copy name is derived server-side. The existing agent.profile.created
// notification lets every open settings surface pick the copy up live.
func (h *Handlers) httpDuplicateProfile(c *gin.Context) {
	profileID := c.Param("id")
	if profileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile id is required"})
		return
	}
	resp, err := h.controller.DuplicateProfile(c.Request.Context(), controller.DuplicateProfileRequest{
		ID: profileID,
	})
	if err != nil {
		if errors.Is(err, controller.ErrAgentFeatureDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		if errors.Is(err, controller.ErrDynamicProfileDuplicationUnsupported) {
			c.JSON(http.StatusConflict, gin.H{"error": "dynamic profiles cannot be duplicated"})
			return
		}
		h.logger.Error("failed to duplicate profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to duplicate profile"})
		return
	}
	h.broadcastProfileEvent(c.Request.Context(), ws.ActionAgentProfileCreated, resp)
	c.JSON(http.StatusOK, resp)
}

// broadcastProfileEvent fans a profile create/update/delete event out.
// Kanban profiles (empty WorkspaceID) go to every settings client.
// Office-scoped profiles are routed through the workspace-scoped broadcaster
// so their configuration (env vars, servers, ...) never leaks across
// workspace/user boundaries — the HTTP agent list hides them via
// filterGlobalProfiles, and the WS path must not contradict that. When the
// hub does not support workspace routing (test fakes), the office event is
// dropped fail-closed.
func (h *Handlers) broadcastProfileEvent(ctx context.Context, action string, profile *dto.AgentProfileDTO) {
	if h.hub == nil {
		return
	}
	// Profile events can arrive before settings-agent hydration. Include the
	// capability needed by sessionless pickers so they need not guess.
	inferenceCapable := false
	if action != ws.ActionAgentProfileDeleted {
		agent, err := h.controller.GetAgent(ctx, profile.AgentID)
		if err != nil {
			h.logger.Warn("failed to load agent capability for profile event", zap.Error(err))
		} else {
			inferenceCapable = agent.InferenceCapable
		}
	}
	notification, _ := ws.NewNotification(action, gin.H{
		"profile":           profile,
		"inference_capable": inferenceCapable,
	})
	if profile.WorkspaceID != "" {
		if workspaceHub, ok := h.hub.(interface {
			BroadcastToWorkspaceOrDrop(string, *ws.Message)
		}); ok {
			workspaceHub.BroadcastToWorkspaceOrDrop(profile.WorkspaceID, notification)
		}
		return
	}
	h.hub.Broadcast(notification)
}

func (h *Handlers) httpDeleteProfile(c *gin.Context) {
	force := c.Query("force") == "true"
	profile, err := h.controller.DeleteProfile(c.Request.Context(), c.Param("id"), force)
	if err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		var inUseErr *controller.ErrProfileInUseDetail
		if errors.As(err, &inUseErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":            "agent profile is in use",
				"active_sessions":  inUseErr.ActiveSessions,
				"watchers":         inUseErr.Watchers,
				"routing_tiers":    inUseErr.RoutingTiers,
				"automations":      inUseErr.Automations,
				"utility_agents":   inUseErr.UtilityAgents,
				"dynamic_profiles": inUseErr.DynamicProfiles,
			})
			return
		}
		h.logger.Error("failed to delete profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete profile"})
		return
	}
	h.broadcastProfileEvent(c.Request.Context(), ws.ActionAgentProfileDeleted, profile)
	c.JSON(http.StatusOK, gin.H{"success": true})
}
