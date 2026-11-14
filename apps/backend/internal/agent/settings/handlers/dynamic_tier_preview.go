package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func (h *Handlers) requireDynamicTiers(c *gin.Context) {
	if !h.controller.DynamicTiersEnabled() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Next()
}

func (h *Handlers) httpPreviewDynamicTiers(c *gin.Context) {
	var request struct {
		Dynamic *dto.DynamicAgentProfileDTO `json:"dynamic"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preview document"})
		return
	}
	if c.Param("id") == "" && request.Dynamic == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dynamic draft is required"})
		return
	}
	result, err := h.controller.PreviewDynamicTiers(c.Request.Context(), c.Param("id"), request.Dynamic)
	if err == nil {
		c.JSON(http.StatusOK, result)
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, controller.ErrDynamicTierPreviewDisabled) {
		status = http.StatusNotFound
	}
	if errors.Is(err, controller.ErrDynamicTierPreviewUnavailable) {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"error": "dynamic selection preview unavailable"})
}
