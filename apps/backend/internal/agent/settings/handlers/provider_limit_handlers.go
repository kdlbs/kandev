package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// httpListProviderLimits returns every provider's limit settings and the reset
// instants routing currently uses.
func (h *Handlers) httpListProviderLimits(c *gin.Context) {
	resp, err := h.controller.ListProviderLimits(c.Request.Context())
	if err != nil {
		h.providerLimitError(c, err, "failed to list provider limits")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// httpUpdateProviderLimit replaces one provider's monthly reset and manual
// block.
func (h *Handlers) httpUpdateProviderLimit(c *gin.Context) {
	var req controller.UpdateProviderLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider limit request"})
		return
	}
	resp, err := h.controller.UpdateProviderLimit(c.Request.Context(), c.Param("provider"), req)
	if err != nil {
		h.providerLimitError(c, err, "failed to update provider limit")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) providerLimitError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, controller.ErrInvalidProviderLimit):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, controller.ErrProviderLimitsUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		h.logger.Error(message, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
	}
}
