package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/usage"
	"net/http"
	"time"
)

type nativeUsageRead func(context.Context, string) ([]usage.ProfileExecutionUsage, error)

// respondNativeProfileUsage runs read for the profile and maps its error to the
// shared response; extra fields are merged into the success body.
func respondNativeProfileUsage(c *gin.Context, read nativeUsageRead, extra gin.H) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	entries, err := read(ctx, c.Param("id"))
	if errors.Is(err, controller.ErrAgentFeatureDisabled) || errors.Is(err, controller.ErrAgentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "usage unavailable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage observation unavailable"})
		return
	}
	body := gin.H{"profile_id": c.Param("id"), "executions": entries}
	for k, v := range extra {
		body[k] = v
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handlers) httpReadNativeProfileUsage(c *gin.Context) {
	respondNativeProfileUsage(c, h.controller.ReadNativeProfileUsage, nil)
}

func (h *Handlers) httpReadCachedNativeProfileUsage(c *gin.Context) {
	respondNativeProfileUsage(c, h.controller.ReadCachedNativeProfileUsage, gin.H{"cached_only": true})
}
