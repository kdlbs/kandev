package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"go.uber.org/zap"
	"net/http"
)

func (h *Handlers) httpSetAutomaticRuntimeUpdates(c *gin.Context) {
	name, ok := requireAgentName(c)
	if !ok {
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeStrictSettingsJSON(c, &request); err != nil || request.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
		return
	}
	err := h.controller.SetAgentAutomaticUpdates(c.Request.Context(), name, *request.Enabled)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"enabled": *request.Enabled})
	case errors.Is(err, controller.ErrAgentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
	case errors.Is(err, controller.ErrRuntimeUpdateUnsupported):
		c.JSON(http.StatusBadRequest, gin.H{"error": "automatic updates unsupported for this runtime"})
	default:
		h.logger.Error("failed to save automatic runtime policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save automatic runtime policy"})
	}
}
