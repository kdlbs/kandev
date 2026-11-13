package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/analytics/models"
)

type recordedModelUsageReader interface {
	GetRecordedModelUsage(context.Context, string, time.Time, time.Time) ([]models.RecordedModelUsage, error)
}

func (h *StatsHandlers) httpGetRecordedModelUsage(c *gin.Context) {
	wsID := c.Param("id")
	if !h.authorize(c, wsID) {
		return
	}
	now := time.Now().UTC()
	hours := 24
	switch c.DefaultQuery("range", "day") {
	case "day":
	case "week":
		hours = 168
	case "month":
		hours = 720
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "ledger range must be day, week or month"})
		return
	}
	reader, ok := h.repo.(recordedModelUsageReader)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recorded model usage unavailable"})
		return
	}
	since := now.Add(-time.Duration(hours) * time.Hour)
	rows, err := reader.GetRecordedModelUsage(c.Request.Context(), wsID, since, now)
	if err != nil {
		h.fail(c, wsID, "recorded model usage", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"basis": "task_usage_events", "workspace_id": wsID, "since": since, "generated_at": now, "account_attribution": "unavailable", "coverage": "recorded_events_only", "truncated": false, "models": rows})
}
