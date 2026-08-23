package api

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/system/metrics"
)

const (
	labelGitPollLatency     = "Git poll latency"
	labelMonitorPollLatency = "Workspace monitor latency"
	labelCreateToReady      = "Create-to-ready"
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
	snapshot.Metrics = s.mergeMetricsInRequestOrder(metricIDs, snapshot.Metrics)
	c.JSON(http.StatusOK, snapshot)
}

// mergeMetricsInRequestOrder interleaves the collector's samples (already in
// the same relative order as the non-diagnostic IDs in metricIDs, since
// collectorMetricIDs preserves that subsequence) with a freshly built
// diagnostic sample per diagnostic ID, walking metricIDs so a mixed request
// like ?metrics=agentctl_goroutines,cpu_percent gets its response back in the
// order it was requested rather than diagnostics always trailing.
func (s *Server) mergeMetricsInRequestOrder(metricIDs []string, collectorSamples []metrics.MetricSample) []metrics.MetricSample {
	out := make([]metrics.MetricSample, 0, len(metricIDs))
	next := 0
	for _, id := range metricIDs {
		if isAgentctlDiagnosticMetric(id) {
			out = append(out, s.agentctlDiagnosticSample(id))
			continue
		}
		if next < len(collectorSamples) {
			out = append(out, collectorSamples[next])
			next++
		}
	}
	return out
}

// collectorMetricIDs filters the agentctl-scoped diagnostic IDs out before
// they reach metrics.Collector.Sample. The collector doesn't recognize them
// and would otherwise emit its own "unknown metric" sample for each one,
// duplicating the correct sample mergeMetricsInRequestOrder builds via
// agentctlDiagnosticSample.
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
	case metrics.MetricAgentctlGoroutines,
		metrics.MetricAgentctlGitPollMillis,
		metrics.MetricAgentctlMonitorMillis,
		metrics.MetricAgentctlCreateReadyMs:
		return true
	default:
		return false
	}
}

// agentctlDiagnosticSample builds the MetricSample for one per-instance
// diagnostic metric ID. These IDs are intentionally absent from
// metrics.isKnownMetric, so they can never enter persisted GlobalSettings or
// the periodic broadcast — this handler is the only path that serves them,
// gated on being named explicitly in the request. Callers must only pass an
// ID that isAgentctlDiagnosticMetric accepts; any other ID returns a
// zero-value sample.
func (s *Server) agentctlDiagnosticSample(id string) metrics.MetricSample {
	switch id {
	case metrics.MetricAgentctlGoroutines:
		return goroutineCountSample()
	case metrics.MetricAgentctlGitPollMillis:
		return s.gitPollLatencySample()
	case metrics.MetricAgentctlMonitorMillis:
		return s.monitorPollLatencySample()
	case metrics.MetricAgentctlCreateReadyMs:
		return s.createReadyMillisSample()
	default:
		return metrics.MetricSample{}
	}
}

// goroutineCountSample reports runtime.NumGoroutine(), which is process-wide:
// when one agentctl process hosts several instances (multiple task sessions
// sharing a Docker container, or host-utility mode), every instance's
// endpoint reports the same number rather than that instance's own share.
// Go has no per-goroutine ownership accounting without invasive
// instrumentation (e.g. pprof.Do at every spawn site across the codebase),
// so this is deliberately process-scoped; correlate it against the live
// instance count from the control server to reason about growth per
// instance.
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

func (s *Server) monitorPollLatencySample() metrics.MetricSample {
	count, meanMillis := s.procMgr.MonitorPollStats()
	if count == 0 {
		return metrics.MetricSample{
			ID:        metrics.MetricAgentctlMonitorMillis,
			Label:     labelMonitorPollLatency,
			Unit:      "ms",
			Available: false,
			Error:     "no workspace monitor scan completed yet",
		}
	}
	return metrics.MetricSample{
		ID:        metrics.MetricAgentctlMonitorMillis,
		Label:     labelMonitorPollLatency,
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
