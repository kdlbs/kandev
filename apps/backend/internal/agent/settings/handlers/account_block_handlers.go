package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// httpReleaseAccountBlock closes the hold a provider account restriction put
// on one execution profile. Repeating it is safe: `released` is false when
// there was no hold to close.
func (h *Handlers) httpReleaseAccountBlock(c *gin.Context) {
	released, err := h.controller.ReleaseAccountBlock(c.Request.Context(), c.Param("id"))
	switch {
	case errors.Is(err, controller.ErrAgentProfileNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
	case errors.Is(err, controller.ErrAccountBlockReleaseUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account-block release is unavailable"})
	case err != nil:
		h.logger.Error("failed to release account block", zap.String("profile_id", c.Param("id")), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to release account block"})
	default:
		c.JSON(http.StatusOK, gin.H{"released": released})
	}
}
