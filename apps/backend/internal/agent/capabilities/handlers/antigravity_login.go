package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handlers) startAntigravityLogin(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host utility not configured"})
		return
	}
	snapshot, err := h.hostUtility.StartAntigravityLogin(c.Request.Context())
	if err != nil {
		h.logger.Warn("antigravity login start failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "antigravity sign-in could not start"})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *Handlers) getAntigravityLogin(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host utility not configured"})
		return
	}
	snapshot, ok := h.hostUtility.GetAntigravityLogin(c.Request.Context(), c.Param("attemptID"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "antigravity sign-in attempt not found"})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *Handlers) cancelAntigravityLogin(c *gin.Context) {
	if h.hostUtility == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "host utility not configured"})
		return
	}
	snapshot, ok := h.hostUtility.CancelAntigravityLogin(c.Request.Context(), c.Param("attemptID"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "antigravity sign-in attempt not found"})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
