package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"go.uber.org/zap"
)

// controlView is the opt-in query value that selects a control projection on
// an existing workspace route.
const controlView = "control"

type controlHistoryRepository interface {
	ReadControlHistory(context.Context, string, time.Time) (*sqlite.ControlHistory, error)
}

// controlHistory serves the opt-in recorded 24-hour event projection on the
// existing workspace activity route; the legacy activity payload is unchanged
// when the caller does not ask for view=control.
func (h *Handler) controlHistory(c *gin.Context) bool {
	if c.Query("view") != controlView {
		return false
	}
	reader, ok := h.labels.(controlHistoryRepository)
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
	history, err := reader.ReadControlHistory(ctx, c.Param("wsId"), time.Now().UTC())
	if err != nil {
		h.logger.Warn("control history read failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control projection unavailable"})
		return true
	}
	c.JSON(http.StatusOK, history)
	return true
}
