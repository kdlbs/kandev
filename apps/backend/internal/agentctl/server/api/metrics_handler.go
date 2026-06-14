package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/system/metrics"
)

func (s *Server) handleSystemMetrics(c *gin.Context) {
	metricIDs := splitMetrics(c.Query("metrics"))
	if len(metricIDs) == 0 {
		metricIDs = metrics.DefaultSettings().Metrics
	}
	diskPath := c.Query("disk_path")
	if diskPath == "" {
		diskPath = "/"
	}
	collector := metrics.NewCollector()
	snapshot := collector.Sample(c.Request.Context(), metricIDs, diskPath)
	snapshot.ID = "agentctl"
	snapshot.Label = "Execution"
	snapshot.Kind = "execution"
	c.JSON(http.StatusOK, snapshot)
}

func splitMetrics(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
