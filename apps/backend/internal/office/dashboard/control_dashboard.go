package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"go.uber.org/zap"
)

type controlDashboardRepository interface {
	ReadControlMetrics(context.Context, string, time.Time) (*sqlite.ControlMetrics, error)
}

// The existing workspace GET retains its authorization and default payload.
func (h *Handler) controlDashboard(c *gin.Context) bool {
	if c.Query("view") != "control" {
		return false
	}
	reader, ok := h.labels.(controlDashboardRepository)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control projection unavailable"})
		return true
	}
	if hours := c.Query("window_hours"); hours != "" && hours != "24" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "window_hours must be 24"})
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	metrics, err := reader.ReadControlMetrics(ctx, c.Param("wsId"), time.Now().UTC())
	if err != nil {
		h.logger.Warn("control dashboard read failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control projection unavailable"})
		return true
	}
	c.JSON(http.StatusOK, metrics)
	return true
}
