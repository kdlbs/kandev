package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"go.uber.org/zap"
)

func (h *Handlers) httpDiscoverAgentMCP(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id is required"})
		return
	}
	discovery, err := h.controller.DiscoverAgentMCP(c.Request.Context(), agentID)
	if err != nil {
		if errors.Is(err, controller.ErrAgentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		h.logger.Error("failed to discover agent MCP servers", zap.String("agent_id", agentID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover agent MCP servers"})
		return
	}
	c.JSON(http.StatusOK, discovery)
}
