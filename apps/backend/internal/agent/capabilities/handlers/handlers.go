// Package handlers exposes the /api/v1/agents/* HTTP routes backed by the
// host utility manager: list cached capabilities, re-probe an agent type, and
// run raw one-off prompts against warm host instances.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/logger"
)

// Handlers holds dependencies for the capability routes.
type Handlers struct {
	hostUtility *hostutility.Manager
	logger      *logger.Logger
}

// RegisterRoutes mounts the capability routes on the given router.
//
// These endpoints sit under `/api/v1/agent-capabilities` rather than under
// `/api/v1/agents/...` to avoid a routing conflict with the existing settings
// handler which uses `/api/v1/agents/:id/...` — Gin does not allow two
// different wildcard names (:id vs :type) at the same path level.
func RegisterRoutes(router *gin.Engine, hostUtility *hostutility.Manager, log *logger.Logger) {
	h := &Handlers{
		hostUtility: hostUtility,
		logger:      log.WithFields(zap.String("component", "agent-capabilities-handlers")),
	}
	api := router.Group("/api/v1/agent-capabilities")
	api.GET("", h.list)
	api.GET("/:type", h.get)
	api.POST("/:type/probe", h.probe)
	api.POST("/:type/prompt", h.prompt)
}

type capabilitiesResponse struct {
	Agents []hostutility.AgentCapabilities `json:"agents"`
}

func (h *Handlers) list(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusOK, capabilitiesResponse{Agents: nil})
		return
	}
	c.JSON(http.StatusOK, capabilitiesResponse{Agents: h.hostUtility.GetAll()})
}

func (h *Handlers) get(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host utility not configured"})
		return
	}
	caps, ok := h.hostUtility.Get(c.Param("type"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent type not found"})
		return
	}
	c.JSON(http.StatusOK, caps)
}

func (h *Handlers) probe(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host utility not configured"})
		return
	}
	agentType := c.Param("type")
	caps, err := h.hostUtility.Refresh(c.Request.Context(), agentType)
	if err != nil {
		h.logger.Warn("probe refresh failed",
			zap.String("agent_type", agentType), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, caps)
}

// promptRequest is the body for POST /api/v1/agents/{type}/prompt.
type promptRequest struct {
	Model  string `json:"model,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Prompt string `json:"prompt" binding:"required"`
}

// promptResponse mirrors hostutility.PromptResult for JSON output.
type promptResponse struct {
	Response       string `json:"response"`
	Model          string `json:"model,omitempty"`
	PromptTokens   int    `json:"prompt_tokens,omitempty"`
	ResponseTokens int    `json:"response_tokens,omitempty"`
	DurationMs     int    `json:"duration_ms,omitempty"`
}

func (h *Handlers) prompt(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host utility not configured"})
		return
	}
	var req promptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	agentType := c.Param("type")
	result, err := h.hostUtility.ExecutePrompt(c.Request.Context(), agentType, req.Model, req.Mode, req.Prompt)
	if err != nil {
		h.logger.Warn("raw prompt failed",
			zap.String("agent_type", agentType), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, promptResponse{
		Response:       result.Response,
		Model:          result.Model,
		PromptTokens:   result.PromptTokens,
		ResponseTokens: result.ResponseTokens,
		DurationMs:     result.DurationMs,
	})
}
