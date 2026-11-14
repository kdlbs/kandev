package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// httpListRoutingCircuits reports the dynamic routing circuits the engine is
// currently acting on. It is read-only: no circuit is opened, closed, retried,
// or reordered by this request.
func (h *Handlers) httpListRoutingCircuits(c *gin.Context) {
	response, err := h.controller.ListRoutingCircuits(c.Request.Context())
	switch {
	case errors.Is(err, controller.ErrRoutingCircuitsUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing circuits are unavailable"})
	case err != nil:
		h.logger.Error("failed to list routing circuits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list routing circuits"})
	default:
		c.JSON(http.StatusOK, response)
	}
}
