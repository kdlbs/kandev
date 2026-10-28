package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
)

func (h *Handlers) httpDiscoverCopilotPlugins(c *gin.Context) {
	agentID := strings.TrimSpace(c.Param("id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id is required"})
		return
	}
	var req dto.CopilotPluginDiscoveryRequest
	if err := decodeStrictSettingsJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin discovery request"})
		return
	}
	scope, ok := requireProfileDiscoveryScope(c)
	if !ok {
		return
	}
	req.AuthorizationScope = scope
	resp, err := h.controller.DiscoverCopilotPlugins(c.Request.Context(), agentID, req)
	if err != nil {
		writePluginDiscoveryError(c, h, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func writePluginDiscoveryError(c *gin.Context, h *Handlers, err error) {
	switch {
	case errors.Is(err, controller.ErrAgentNotFound), errors.Is(err, controller.ErrProfileDiscoveryMissing):
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin discovery target not found"})
	case errors.Is(err, controller.ErrInvalidProfileDiscovery):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin discovery context"})
	case errors.Is(err, controller.ErrCopilotPluginDiscoverySuperseded):
		c.JSON(http.StatusConflict, gin.H{"error": "plugin discovery superseded"})
	default:
		h.logger.Error("failed to discover copilot plugins", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover copilot plugins"})
	}
}
