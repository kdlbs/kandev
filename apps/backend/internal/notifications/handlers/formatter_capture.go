package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/notifications/service"
)

func (h *Handlers) httpFormatterCapture(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	identity, ok := authn.FromGin(c)
	if !ok || !identity.IsAdmin() {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		return
	}
	id := c.Param("id")
	if id == "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		lease, err := h.controller.StartFormatterCapture(ctx)
		if err != nil {
			writeFormatterError(c, err)
			return
		}
		c.JSON(http.StatusOK, lease)
		return
	}
	if c.Request.Method == http.MethodGet {
		lease, err := h.controller.FormatterCapture(c.Request.Context(), id)
		if err != nil {
			writeCaptureLeaseError(c, err)
			return
		}
		c.JSON(http.StatusOK, lease)
		return
	}
	if err := h.controller.ReleaseFormatterCapture(c.Request.Context(), id); err != nil {
		writeCaptureLeaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"released": true})
}

func writeCaptureLeaseError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrFormatterCaptureNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification capture lease not found"})
		return
	}
	writeFormatterError(c, err)
}
