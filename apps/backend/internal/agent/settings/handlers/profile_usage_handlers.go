package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

func (h *Handlers) httpListProfileUsage(c *gin.Context) {
	result, err := h.controller.ListProfileUsage(c.Request.Context())
	switch {
	case errors.Is(err, controller.ErrProviderUsageDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": "provider usage snapshots are disabled"})
	case err != nil:
		// A repository or provider error may contain private account details.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider usage snapshots are unavailable"})
	default:
		c.JSON(http.StatusOK, result)
	}
}
