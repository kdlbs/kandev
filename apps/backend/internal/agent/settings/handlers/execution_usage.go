package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"net/http"
	"time"
)

func (h *Handlers) httpReadNativeProfileUsage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	entries, err := h.controller.ReadNativeProfileUsage(ctx, c.Param("id"))
	if errors.Is(err, controller.ErrAgentFeatureDisabled) || errors.Is(err, controller.ErrAgentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "usage unavailable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage observation unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile_id": c.Param("id"), "executions": entries})
}
