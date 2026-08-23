package api

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/system/metrics"
)

const (
	labelGitPollLatency = "Git poll latency"
	labelCreateToReady  = "Create-to-ready"
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
	snapshot := s.metricsCollector.Sample(c.Request.Context(), collectorMetricIDs(metricIDs), diskPath)
	snapshot.ID = "agentctl"
	snapshot.Label = "Execution"
	snapshot.Kind = "execution"
	snapshot.Metrics = append(snapshot.Metrics, s.agentctlDiagnosticSamples(metricIDs)...)
	c.JSON(http.StatusOK, snapshot)
}

// collectorMetricIDs filters the agentctl-scoped diagnostic IDs out before
// they reach metrics.Collector.Sample. The collector doesn't recognize them
// and would otherwise emit its own "unknown metric" sample for each one,
// duplicating the correct sample agentctlDiagnosticSamples appends below.
func collectorMetricIDs(metricIDs []string) []string {
	out := make([]string, 0, len(metricIDs))
	for _, id := range metricIDs {
		if !isAgentctlDiagnosticMetric(id) {
			out = append(out, id)
		}
	}
	return out
}

func isAgentctlDiagnosticMetric(id string) bool {
	switch id {
	case metrics.MetricAgentctlGoroutines, metrics.MetricAgentctlGitPollMillis, metrics.MetricAgentctlCreateReadyMs:
		return true
	default:
		return false
	}
}

// agentctlDiagnosticSamples builds MetricSample entries for the per-instance
// diagnostic metric IDs present in metricIDs. These IDs are intentionally
// absent from metrics.isKnownMetric, so they can never enter persisted
// GlobalSettings or the periodic broadcast — this handler is the only path
// that serves them, gated on being named explicitly in the request.
func (s *Server) agentctlDiagnosticSamples(metricIDs []string) []metrics.MetricSample {
	var out []metrics.MetricSample
	for _, id := range metricIDs {
		switch id {
		case metrics.MetricAgentctlGoroutines:
			out = append(out, goroutineCountSample())
		case metrics.MetricAgentctlGitPollMillis:
			out = append(out, s.gitPollLatencySample())
		case metrics.MetricAgentctlCreateReadyMs:
			out = append(out, s.createReadyMillisSample())
		}
	}
	return out
}

func goroutineCountSample() metrics.MetricSample {
	value := float64(runtime.NumGoroutine())
	return metrics.MetricSample{
		ID:        metrics.MetricAgentctlGoroutines,
		Label:     "Goroutines",
		Available: true,
		Value:     &value,
	}
}

func (s *Server) gitPollLatencySample() metrics.MetricSample {
	count, meanMillis := s.procMgr.GitPollStats()
	if count == 0 {
		return metrics.MetricSample{
			ID:        metrics.MetricAgentctlGitPollMillis,
			Label:     labelGitPollLatency,
			Unit:      "ms",
			Available: false,
			Error:     "no git poll tick completed yet",
		}
	}
	return metrics.MetricSample{
		ID:        metrics.MetricAgentctlGitPollMillis,
		Label:     labelGitPollLatency,
		Unit:      "ms",
		Available: true,
		Value:     &meanMillis,
	}
}

func (s *Server) createReadyMillisSample() metrics.MetricSample {
	if s.cfg.CreateReadyMillis == nil {
		return metrics.MetricSample{
			ID:        metrics.MetricAgentctlCreateReadyMs,
			Label:     labelCreateToReady,
			Unit:      "ms",
			Available: false,
			Error:     "creation duration not tracked for this instance",
		}
	}
	millis := s.cfg.CreateReadyMillis.Load()
	if millis == 0 {
		return metrics.MetricSample{
			ID:        metrics.MetricAgentctlCreateReadyMs,
			Label:     labelCreateToReady,
			Unit:      "ms",
			Available: false,
			Error:     "instance still starting",
		}
	}
	value := float64(millis)
	return metrics.MetricSample{
		ID:        metrics.MetricAgentctlCreateReadyMs,
		Label:     labelCreateToReady,
		Unit:      "ms",
		Available: true,
		Value:     &value,
	}
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
