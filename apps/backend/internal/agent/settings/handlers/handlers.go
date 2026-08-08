package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

var availableAgentsBroadcastTimeout = 10 * time.Second

type Handlers struct {
	controller *controller.Controller
	hub        Broadcaster
	logger     *logger.Logger
	interlock  gin.HandlerFunc
}

type Broadcaster interface {
	Broadcast(msg *ws.Message)
}

func NewHandlers(ctrl *controller.Controller, hub Broadcaster, log *logger.Logger, interlockToken string) *Handlers {
	return &Handlers{
		controller: ctrl,
		hub:        hub,
		logger:     log.WithFields(zap.String("component", "agent-settings-handlers")),
		interlock:  httpmw.InterimSettingsInterlock(interlockToken),
	}
}

func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, hub Broadcaster, log *logger.Logger, interlockToken string) {
	// Wire the install job store with the same broadcaster used by other
	// agent-settings notifications so streaming install events reach the UI.
	ctrl.SetJobBroadcaster(hub)
	handlers := NewHandlers(ctrl, hub, log, interlockToken)
	handlers.registerHTTP(router)
}

func (h *Handlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/agents/discovery", h.httpDiscoverAgents)
	api.GET("/agents/available", h.httpListAvailableAgents)
	api.GET("/agents", h.httpListAgents)
	api.POST("/agents", h.interlock, h.httpCreateAgent)
	api.POST("/agents/tui", h.interlock, h.httpCreateCustomTUIAgent)
	api.GET("/agents/:id", h.httpGetAgent)
	api.PATCH("/agents/:id", h.interlock, h.httpUpdateAgent)
	api.DELETE("/agents/:id", h.interlock, h.httpDeleteAgent)
	api.POST("/agents/:id/profiles", h.interlock, h.httpCreateProfile)
	api.GET("/agents/:id/logo", h.httpGetAgentLogo)
	api.GET("/agent-models/:agentName", h.httpGetAgentModels)
	api.POST("/agent-models/:agentName/resolve", h.httpResolveAgentModelConfig)
	api.POST("/agent-command-preview/:agentName", h.httpPreviewAgentCommand)
	api.POST("/agent-install/:agentName", h.interlock, h.httpInstallAgent)
	api.GET("/agent-update/:agentName/preview", h.httpPreviewAgentUpdate)
	api.GET("/agent-install/jobs", h.httpListInstallJobs)
	api.GET("/agent-install/jobs/:id", h.httpGetInstallJob)
	api.POST("/agent-update/:agentName", h.interlock, h.httpUpdateAgentRuntime)
	api.GET("/agent-update/jobs", h.httpListAgentUpdateJobs)
	api.GET("/agent-update/jobs/:id", h.httpGetAgentUpdateJob)
	api.PATCH("/agent-profiles/:id", h.interlock, h.httpUpdateProfile)
	api.DELETE("/agent-profiles/:id", h.interlock, h.httpDeleteProfile)
	api.GET("/agent-profiles/:id/mcp-config", h.httpGetProfileMcpConfig)
	api.POST("/agent-profiles/:id/mcp-config", h.interlock, h.httpUpdateProfileMcpConfig)
}

func (h *Handlers) httpDiscoverAgents(c *gin.Context) {
	h.controller.InvalidateDiscoveryCache()
	resp, err := h.controller.ListDiscovery(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to discover agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover agents"})
		return
	}
	c.JSON(http.StatusOK, resp)
	h.broadcastAvailableAgentsAsync()
}

func (h *Handlers) httpListAvailableAgents(c *gin.Context) {
	resp, err := h.controller.ListAvailableAgents(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list available agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list available agents"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// httpInstallAgent enqueues an async install and returns the job snapshot.
// The install runs in a goroutine; clients track progress via WS
// (agent.install.started/output/finished) or by polling /agent-install/jobs/:id.
//
// Idempotent: a second POST for the same agent while a job is running returns
// the existing job_id rather than starting a duplicate.
func (h *Handlers) httpInstallAgent(c *gin.Context) {
	name, ok := requireAgentName(c)
	if !ok {
		return
	}
	h.enqueueMaintenance(c, name, "install", func() (any, error) {
		return h.controller.EnqueueInstall(name)
	}, classifyInstallError)
}

func (h *Handlers) httpUpdateAgentRuntime(c *gin.Context) {
	name, ok := requireAgentName(c)
	if !ok {
		return
	}
	h.enqueueMaintenance(c, name, "update", func() (any, error) {
		return h.controller.EnqueueAgentUpdate(name)
	}, classifyUpdateError)
}

func (h *Handlers) httpPreviewAgentUpdate(c *gin.Context) {
	name, ok := requireAgentName(c)
	if !ok {
		return
	}
	preview, err := h.controller.PreviewAgentUpdate(c.Request.Context(), name)
	if err == nil {
		c.JSON(http.StatusOK, preview)
		return
	}
	if status, message, matched := classifyUpdatePreviewError(err); matched {
		c.JSON(status, gin.H{"error": message})
		return
	}
	h.logger.Error("failed to preview agent runtime update", zap.String("agent", name), zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to preview agent update"})
}

func requireAgentName(c *gin.Context) (string, bool) {
	name := strings.TrimSpace(c.Param("agentName"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return "", false
	}
	return name, true
}

func writeMaintenanceConflictIfPresent(c *gin.Context, err error) bool {
	var conflict *controller.MaintenanceConflictError
	if !errors.As(err, &conflict) {
		return false
	}
	writeMaintenanceConflict(c, conflict)
	return true
}

type maintenanceErrorClassifier func(error) (int, string, bool)

func (h *Handlers) enqueueMaintenance(
	c *gin.Context,
	name string,
	kind string,
	enqueue func() (any, error),
	classify maintenanceErrorClassifier,
) {
	job, err := enqueue()
	if err == nil {
		c.JSON(http.StatusAccepted, job)
		return
	}
	if writeMaintenanceConflictIfPresent(c, err) {
		return
	}
	if status, message, matched := classify(err); matched {
		c.JSON(status, gin.H{"error": message})
		return
	}
	h.logger.Error("failed to enqueue agent maintenance",
		zap.String("agent", name), zap.String("kind", kind), zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue agent " + kind})
}

func classifyInstallError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, controller.ErrAgentNotFound):
		return http.StatusNotFound, "agent not found", true
	case errors.Is(err, controller.ErrInstallScriptEmpty):
		return http.StatusBadRequest, "agent has no install script", true
	case errors.Is(err, controller.ErrJobStoreUnavailable):
		return http.StatusServiceUnavailable, "install service not ready", true
	default:
		return 0, "", false
	}
}

func classifyUpdateError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, controller.ErrAgentNotFound):
		return http.StatusNotFound, "agent not found", true
	case errors.Is(err, controller.ErrRuntimeUpdateUnsupported):
		return http.StatusBadRequest, "agent runtime update unsupported", true
	case errors.Is(err, controller.ErrRuntimeUpdaterUnavailable):
		return http.StatusServiceUnavailable, "agent update service not ready", true
	default:
		return 0, "", false
	}
}

func classifyUpdatePreviewError(err error) (int, string, bool) {
	if status, message, matched := classifyUpdateError(err); matched {
		return status, message, true
	}
	if errors.Is(err, controller.ErrRuntimeUpdatePreviewFailed) {
		return http.StatusBadGateway, "unable to resolve latest runtime version", true
	}
	return 0, "", false
}

func (h *Handlers) httpListAgentUpdateJobs(c *gin.Context) {
	c.JSON(http.StatusOK, dto.ListAgentUpdateJobsResponse{Jobs: h.controller.ListAgentUpdateJobs()})
}

func (h *Handlers) httpGetAgentUpdateJob(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id is required"})
		return
	}
	job, ok := h.controller.GetAgentUpdateJob(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func writeMaintenanceConflict(c *gin.Context, conflict *controller.MaintenanceConflictError) {
	c.JSON(http.StatusConflict, gin.H{
		"error":         conflict.Error(),
		"active_job_id": conflict.Active.JobID,
		"active_kind":   conflict.Active.Kind,
	})
}

func (h *Handlers) httpListInstallJobs(c *gin.Context) {
	c.JSON(http.StatusOK, dto.ListInstallJobsResponse{Jobs: h.controller.ListInstallJobs()})
}

func (h *Handlers) httpGetInstallJob(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id is required"})
		return
	}
	job, ok := h.controller.GetInstallJob(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *Handlers) broadcastAvailableAgentsAsync() {
	if h.hub == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), availableAgentsBroadcastTimeout)
		defer cancel()

		availableResp, err := h.controller.ListAvailableAgents(ctx)
		if err != nil {
			h.logger.Error("failed to list available agents for broadcast", zap.Error(err))
			return
		}
		notification, _ := ws.NewNotification(ws.ActionAgentAvailableUpdated, gin.H{
			"agents": availableResp.Agents,
		})
		h.hub.Broadcast(notification)
	}()
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
	Name          string                 `json:"name"`
	Model         string                 `json:"model"`
	Mode          string                 `json:"mode,omitempty"`
	CLIFlags      []dto.CLIFlagDTO       `json:"cli_flags,omitempty"`
	EnvVars       []dto.ProfileEnvVarDTO `json:"env_vars,omitempty"`
	CommandPrefix string                 `json:"command_prefix,omitempty"`
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
		if strings.TrimSpace(profile.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "profile name is required"})
			return
		}
		profiles = append(profiles, controller.CreateAgentProfileRequest{
			Name:          profile.Name,
			Model:         profile.Model,
			Mode:          profile.Mode,
			CLIFlags:      profile.CLIFlags,
			EnvVars:       profile.EnvVars,
			CommandPrefix: profile.CommandPrefix,
		})
	}
	resp, err := h.controller.CreateAgent(c.Request.Context(), controller.CreateAgentRequest{
		Name:        body.Name,
		WorkspaceID: body.WorkspaceID,
		Profiles:    profiles,
	})
	if err != nil {
		if errors.Is(err, controller.ErrInvalidProfileEnvVars) || errors.Is(err, controller.ErrInvalidCommandPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
	h.broadcastAvailableAgentsAsync()
}

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
}

type createProfileRequest struct {
	Name           string                 `json:"name"`
	Model          string                 `json:"model"`
	Mode           string                 `json:"mode,omitempty"`
	ConfigOptions  map[string]string      `json:"config_options,omitempty"`
	AllowIndexing  bool                   `json:"allow_indexing"`
	AutoApprove    bool                   `json:"auto_approve"`
	CLIPassthrough bool                   `json:"cli_passthrough"`
	CLIFlags       []dto.CLIFlagDTO       `json:"cli_flags,omitempty"`
	EnvVars        []dto.ProfileEnvVarDTO `json:"env_vars,omitempty"`
	CommandPrefix  string                 `json:"command_prefix,omitempty"`
}

func (h *Handlers) httpCreateProfile(c *gin.Context) {
	var body createProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile name is required"})
		return
	}
	resp, err := h.controller.CreateProfile(c.Request.Context(), controller.CreateProfileRequest{
		AgentID:        c.Param("id"),
		Name:           body.Name,
		Model:          body.Model,
		Mode:           body.Mode,
		ConfigOptions:  body.ConfigOptions,
		AllowIndexing:  body.AllowIndexing,
		AutoApprove:    body.AutoApprove,
		CLIPassthrough: body.CLIPassthrough,
		CLIFlags:       body.CLIFlags,
		EnvVars:        body.EnvVars,
		CommandPrefix:  body.CommandPrefix,
	})
	if err != nil {
		if errors.Is(err, controller.ErrInvalidProfileEnvVars) || errors.Is(err, controller.ErrInvalidCommandPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileCreated, gin.H{
			"profile": resp,
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, resp)
}

type updateProfileRequest struct {
	Name           *string                 `json:"name,omitempty"`
	Model          *string                 `json:"model,omitempty"`
	Mode           *string                 `json:"mode,omitempty"`
	ConfigOptions  *map[string]string      `json:"config_options,omitempty"`
	AllowIndexing  *bool                   `json:"allow_indexing,omitempty"`
	AutoApprove    *bool                   `json:"auto_approve,omitempty"`
	CLIPassthrough *bool                   `json:"cli_passthrough,omitempty"`
	Enabled        *bool                   `json:"enabled,omitempty"`
	CLIFlags       *[]dto.CLIFlagDTO       `json:"cli_flags,omitempty"`
	EnvVars        *[]dto.ProfileEnvVarDTO `json:"env_vars,omitempty"`
	CommandPrefix  *string                 `json:"command_prefix,omitempty"`
}

func (h *Handlers) httpUpdateProfile(c *gin.Context) {
	var body updateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile name is required"})
		return
	}
	resp, err := h.controller.UpdateProfile(c.Request.Context(), controller.UpdateProfileRequest{
		ID:             c.Param("id"),
		Name:           body.Name,
		Model:          body.Model,
		Mode:           body.Mode,
		ConfigOptions:  body.ConfigOptions,
		AllowIndexing:  body.AllowIndexing,
		AutoApprove:    body.AutoApprove,
		CLIPassthrough: body.CLIPassthrough,
		Enabled:        body.Enabled,
		CLIFlags:       body.CLIFlags,
		EnvVars:        body.EnvVars,
		CommandPrefix:  body.CommandPrefix,
	})
	if err != nil {
		if err == controller.ErrAgentProfileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
			return
		}
		if errors.Is(err, controller.ErrInvalidProfileEnvVars) || errors.Is(err, controller.ErrInvalidCommandPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to update profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileUpdated, gin.H{
			"profile": resp,
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, resp)
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
				"error":           "agent profile is in use",
				"active_sessions": inUseErr.ActiveSessions,
				"watchers":        inUseErr.Watchers,
				"routing_tiers":   inUseErr.RoutingTiers,
				"automations":     inUseErr.Automations,
			})
			return
		}
		h.logger.Error("failed to delete profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete profile"})
		return
	}
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentProfileDeleted, gin.H{
			"profile": profile,
		})
		h.hub.Broadcast(notification)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type commandPreviewRequest struct {
	Model              string           `json:"model"`
	PermissionSettings map[string]bool  `json:"permission_settings"`
	CLIPassthrough     bool             `json:"cli_passthrough"`
	CLIFlags           []dto.CLIFlagDTO `json:"cli_flags"`
	CommandPrefix      string           `json:"command_prefix,omitempty"`
}

func (h *Handlers) httpPreviewAgentCommand(c *gin.Context) {
	agentName := c.Param("agentName")
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}

	var body commandPreviewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	resp, err := h.controller.PreviewAgentCommand(c.Request.Context(), agentName, controller.CommandPreviewRequest{
		Model:              body.Model,
		PermissionSettings: body.PermissionSettings,
		CLIPassthrough:     body.CLIPassthrough,
		CLIFlags:           body.CLIFlags,
		CommandPrefix:      body.CommandPrefix,
	})
	if err != nil {
		h.logger.Error("failed to preview agent command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// httpGetAgentLogo serves agent SVG logos.
// Note: :id here is the agent name (e.g. "claude-code"), not the UUID used by other /agents/:id routes.
func (h *Handlers) httpGetAgentLogo(c *gin.Context) {
	agentName := c.Param("id")
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}

	variant := agents.LogoLight
	if c.Query("variant") == "dark" {
		variant = agents.LogoDark
	}

	data, err := h.controller.GetAgentLogo(c.Request.Context(), agentName, variant)
	if err != nil {
		if err == controller.ErrAgentNotFound || err == controller.ErrLogoNotAvailable {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to get agent logo", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agent logo"})
		return
	}

	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/svg+xml", data)
}

func (h *Handlers) httpGetAgentModels(c *gin.Context) {
	agentName := c.Param("agentName")
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}

	// Check for refresh query parameter
	refresh := c.Query("refresh") == "true"

	resp, err := h.controller.FetchDynamicModels(c.Request.Context(), agentName, refresh)
	if err != nil {
		if errors.Is(err, controller.ErrAgentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to fetch agent models", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpResolveAgentModelConfig(c *gin.Context) {
	agentName := strings.TrimSpace(c.Param("agentName"))
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}
	var req dto.ResolveAgentModelConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model resolution request"})
		return
	}
	resp, err := h.controller.ResolveAgentModelConfig(c.Request.Context(), agentName, req)
	if err != nil {
		if errors.Is(err, controller.ErrModelRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
			return
		}
		if errors.Is(err, controller.ErrAgentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to resolve agent model options", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve agent model options"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
