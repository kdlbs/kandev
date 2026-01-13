package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type Handlers struct {
	controller *controller.Controller
	hub        Broadcaster
	logger     *logger.Logger
}

type Broadcaster interface {
	Broadcast(msg *ws.Message)
}

func NewHandlers(ctrl *controller.Controller, hub Broadcaster, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		hub:        hub,
		logger:     log.WithFields(zap.String("component", "agent-settings-handlers")),
	}
}

func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, hub Broadcaster, log *logger.Logger) {
	handlers := NewHandlers(ctrl, hub, log)
	handlers.registerHTTP(router)
}

func (h *Handlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/agents/discovery", h.httpDiscoverAgents)
	api.GET("/agents", h.httpListAgents)
	api.POST("/agents", h.httpCreateAgent)
	api.GET("/agents/:id", h.httpGetAgent)
	api.PATCH("/agents/:id", h.httpUpdateAgent)
	api.DELETE("/agents/:id", h.httpDeleteAgent)
	api.POST("/agents/:id/profiles", h.httpCreateProfile)
	api.PATCH("/agent-profiles/:id", h.httpUpdateProfile)
	api.DELETE("/agent-profiles/:id", h.httpDeleteProfile)
}

func (h *Handlers) httpDiscoverAgents(c *gin.Context) {
	resp, err := h.controller.ListDiscovery(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to discover agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover agents"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpListAgents(c *gin.Context) {
	resp, err := h.controller.ListAgents(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type createAgentRequest struct {
	Name        string                      `json:"name"`
	WorkspaceID *string                     `json:"workspace_id,omitempty"`
	Profiles    []createAgentProfileRequest `json:"profiles,omitempty"`
}

type createAgentProfileRequest struct {
	Name                       string `json:"name"`
	Model                      string `json:"model"`
	AutoApprove                bool   `json:"auto_approve"`
	DangerouslySkipPermissions bool   `json:"dangerously_skip_permissions"`
	Plan                       string `json:"plan"`
}

func (h *Handlers) httpCreateAgent(c *gin.Context) {
	var body createAgentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	profiles := make([]controller.CreateAgentProfileRequest, 0, len(body.Profiles))
	for _, profile := range body.Profiles {
		profiles = append(profiles, controller.CreateAgentProfileRequest{
			Name:                       profile.Name,
			Model:                      profile.Model,
			AutoApprove:                profile.AutoApprove,
			DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
			Plan:                       profile.Plan,
		})
	}
	resp, err := h.controller.CreateAgent(c.Request.Context(), controller.CreateAgentRequest{
		Name:        body.Name,
		WorkspaceID: body.WorkspaceID,
		Profiles:    profiles,
	})
	if err != nil {
		h.logger.Error("failed to create agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetAgent(c *gin.Context) {
	resp, err := h.controller.GetAgent(c.Request.Context(), c.Param("id"))
	if err != nil {
		if err == controller.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to get agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agent"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type updateAgentRequest struct {
	WorkspaceID   *string `json:"workspace_id,omitempty"`
	SupportsMCP   *bool   `json:"supports_mcp,omitempty"`
	MCPConfigPath *string `json:"mcp_config_path,omitempty"`
}

func (h *Handlers) httpUpdateAgent(c *gin.Context) {
	var body updateAgentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateAgent(c.Request.Context(), controller.UpdateAgentRequest{
		ID:            c.Param("id"),
		WorkspaceID:   body.WorkspaceID,
		SupportsMCP:   body.SupportsMCP,
		MCPConfigPath: body.MCPConfigPath,
	})
	if err != nil {
		if err == controller.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to update agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update agent"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeleteAgent(c *gin.Context) {
	if err := h.controller.DeleteAgent(c.Request.Context(), c.Param("id")); err != nil {
		if err == controller.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to delete agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete agent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type createProfileRequest struct {
	Name                       string `json:"name"`
	Model                      string `json:"model"`
	AutoApprove                bool   `json:"auto_approve"`
	DangerouslySkipPermissions bool   `json:"dangerously_skip_permissions"`
	Plan                       string `json:"plan"`
}

func (h *Handlers) httpCreateProfile(c *gin.Context) {
	var body createProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreateProfile(c.Request.Context(), controller.CreateProfileRequest{
		AgentID:                    c.Param("id"),
		Name:                       body.Name,
		Model:                      body.Model,
		AutoApprove:                body.AutoApprove,
		DangerouslySkipPermissions: body.DangerouslySkipPermissions,
		Plan:                       body.Plan,
	})
	if err != nil {
		h.logger.Error("failed to create profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileCreated, gin.H{
			"profile_id": resp.ID,
			"agent_id":   resp.AgentID,
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, resp)
}

type updateProfileRequest struct {
	Name                       *string `json:"name,omitempty"`
	Model                      *string `json:"model,omitempty"`
	AutoApprove                *bool   `json:"auto_approve,omitempty"`
	DangerouslySkipPermissions *bool   `json:"dangerously_skip_permissions,omitempty"`
	Plan                       *string `json:"plan,omitempty"`
}

func (h *Handlers) httpUpdateProfile(c *gin.Context) {
	var body updateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateProfile(c.Request.Context(), controller.UpdateProfileRequest{
		ID:                         c.Param("id"),
		Name:                       body.Name,
		Model:                      body.Model,
		AutoApprove:                body.AutoApprove,
		DangerouslySkipPermissions: body.DangerouslySkipPermissions,
		Plan:                       body.Plan,
	})
	if err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		h.logger.Error("failed to update profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileUpdated, gin.H{
			"profile_id": resp.ID,
			"agent_id":   resp.AgentID,
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeleteProfile(c *gin.Context) {
	if err := h.controller.DeleteProfile(c.Request.Context(), c.Param("id")); err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		h.logger.Error("failed to delete profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileDeleted, gin.H{
			"profile_id": c.Param("id"),
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
