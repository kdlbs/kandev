package api

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/errors"
	"github.com/kandev/kandev/internal/common/logger"
)

// DockerClient interface for container operations needed by handlers
type DockerClient interface {
	GetContainerLogs(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error)
}

// Handler contains HTTP handlers for the agent manager API
type Handler struct {
	lifecycle *lifecycle.Manager
	registry  *registry.Registry
	docker    DockerClient
	logger    *logger.Logger
}

// NewHandler creates a new API handler
func NewHandler(
	lm *lifecycle.Manager,
	reg *registry.Registry,
	docker DockerClient,
	log *logger.Logger,
) *Handler {
	return &Handler{
		lifecycle: lm,
		registry:  reg,
		docker:    docker,
		logger:    log.WithFields(zap.String("component", "agent-api")),
	}
}

// LaunchAgent launches a new agent for a task
// POST /api/v1/agents/launch
func (h *Handler) LaunchAgent(c *gin.Context) {
	var req LaunchAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.BadRequest("invalid request body: " + err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	launchReq := &lifecycle.LaunchRequest{
		TaskID:        req.TaskID,
		AgentType:     req.AgentType,
		WorkspacePath: req.WorkspacePath,
		Env:           req.Env,
		Metadata:      req.Metadata,
	}

	instance, err := h.lifecycle.Launch(c.Request.Context(), launchReq)
	if err != nil {
		h.logger.Error("failed to launch agent", zap.Error(err))
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "disabled") {
			appErr := errors.BadRequest(err.Error())
			c.JSON(appErr.HTTPStatus, appErr)
			return
		}
		if strings.Contains(err.Error(), "already has an agent running") {
			appErr := errors.Conflict(err.Error())
			c.JSON(appErr.HTTPStatus, appErr)
			return
		}
		appErr := errors.InternalError("failed to launch agent", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := instanceToResponse(instance)
	c.JSON(http.StatusCreated, resp)
}

// StopAgent stops an agent instance
// DELETE /api/v1/agents/:instanceId
func (h *Handler) StopAgent(c *gin.Context) {
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		appErr := errors.BadRequest("instanceId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req StopAgentRequest
	// Bind JSON body if present, but don't require it
	_ = c.ShouldBindJSON(&req)

	err := h.lifecycle.StopAgent(c.Request.Context(), instanceID, req.Force)
	if err != nil {
		h.logger.Error("failed to stop agent", zap.String("instance_id", instanceID), zap.Error(err))
		if strings.Contains(err.Error(), "not found") {
			appErr := errors.NotFound("agent instance", instanceID)
			c.JSON(appErr.HTTPStatus, appErr)
			return
		}
		appErr := errors.InternalError("failed to stop agent", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "agent stopped successfully"})
}

// GetAgentStatus returns agent instance status
// GET /api/v1/agents/:instanceId/status
func (h *Handler) GetAgentStatus(c *gin.Context) {
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		appErr := errors.BadRequest("instanceId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	instance, found := h.lifecycle.GetInstance(instanceID)
	if !found {
		appErr := errors.NotFound("agent instance", instanceID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := instanceToResponse(instance)
	c.JSON(http.StatusOK, resp)
}

// GetAgentLogs returns agent logs
// GET /api/v1/agents/:instanceId/logs
func (h *Handler) GetAgentLogs(c *gin.Context) {
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		appErr := errors.BadRequest("instanceId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	instance, found := h.lifecycle.GetInstance(instanceID)
	if !found {
		appErr := errors.NotFound("agent instance", instanceID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Get tail parameter, default to 100 lines
	tail := c.DefaultQuery("tail", "100")

	if h.docker == nil {
		// Docker client not available, return empty logs
		c.JSON(http.StatusOK, LogsResponse{
			Logs:  []LogEntry{},
			Total: 0,
		})
		return
	}

	reader, err := h.docker.GetContainerLogs(c.Request.Context(), instance.ContainerID, false, tail)
	if err != nil {
		h.logger.Error("failed to get container logs",
			zap.String("instance_id", instanceID),
			zap.String("container_id", instance.ContainerID),
			zap.Error(err))
		appErr := errors.InternalError("failed to get agent logs", err)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}
	defer reader.Close()

	// Parse the logs - Docker log format has 8-byte header
	logs := []LogEntry{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip Docker header bytes if present
		if len(line) > 8 {
			line = line[8:]
		}
		logs = append(logs, LogEntry{
			Timestamp: time.Now(), // Container logs include timestamp
			Message:   line,
			Stream:    "stdout",
		})
	}

	c.JSON(http.StatusOK, LogsResponse{
		Logs:  logs,
		Total: len(logs),
	})
}

// ListAgents returns all active agent instances
// GET /api/v1/agents
func (h *Handler) ListAgents(c *gin.Context) {
	instances := h.lifecycle.ListInstances()

	agents := make([]AgentInstanceResponse, 0, len(instances))
	for _, instance := range instances {
		agents = append(agents, instanceToResponse(instance))
	}

	c.JSON(http.StatusOK, AgentsListResponse{
		Agents: agents,
		Total:  len(agents),
	})
}

// ListAgentTypes returns available agent types
// GET /api/v1/agents/types
func (h *Handler) ListAgentTypes(c *gin.Context) {
	configs := h.registry.List()

	types := make([]AgentTypeResponse, 0, len(configs))
	for _, cfg := range configs {
		types = append(types, AgentTypeResponse{
			ID:           cfg.ID,
			Name:         cfg.Name,
			Description:  cfg.Description,
			Image:        cfg.Image,
			Capabilities: cfg.Capabilities,
			Enabled:      cfg.Enabled,
		})
	}

	c.JSON(http.StatusOK, AgentTypesListResponse{
		Types: types,
		Total: len(types),
	})
}

// GetAgentType returns a specific agent type
// GET /api/v1/agents/types/:typeId
func (h *Handler) GetAgentType(c *gin.Context) {
	typeID := c.Param("typeId")
	if typeID == "" {
		appErr := errors.BadRequest("typeId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	cfg, err := h.registry.Get(typeID)
	if err != nil {
		appErr := errors.NotFound("agent type", typeID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	resp := AgentTypeResponse{
		ID:           cfg.ID,
		Name:         cfg.Name,
		Description:  cfg.Description,
		Image:        cfg.Image,
		Capabilities: cfg.Capabilities,
		Enabled:      cfg.Enabled,
	}

	c.JSON(http.StatusOK, resp)
}

// HealthCheck returns health status
// GET /health
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	})
}

// instanceToResponse converts an AgentInstance to an AgentInstanceResponse
func instanceToResponse(instance *lifecycle.AgentInstance) AgentInstanceResponse {
	return AgentInstanceResponse{
		ID:           instance.ID,
		TaskID:       instance.TaskID,
		AgentType:    instance.AgentType,
		ContainerID:  instance.ContainerID,
		Status:       string(instance.Status),
		Progress:     instance.Progress,
		StartedAt:    instance.StartedAt,
		FinishedAt:   instance.FinishedAt,
		ExitCode:     instance.ExitCode,
		ErrorMessage: instance.ErrorMessage,
		Metadata:     instance.Metadata,
	}
}

