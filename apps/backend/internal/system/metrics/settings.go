package metrics

import (
	"errors"
	"fmt"
)

var ErrValidation = errors.New("metrics settings validation")

const (
	MetricCPUPercent    = "cpu_percent"
	MetricMemoryPercent = "memory_percent"
	MetricDiskPercent   = "disk_percent"
	MetricCPUTemp       = "cpu_temp"
	MetricIOLoad        = "io_load"

	DefaultIntervalSeconds = 5
	MinIntervalSeconds     = 1
	MaxIntervalSeconds     = 5 * 60
)

// Diagnostic per-instance agentctl metrics. These are deliberately NOT
// registered in isKnownMetric, so NormalizeSettings rejects them and they can
// never enter persisted GlobalSettings — which means they can never reach the
// periodic broadcast or the status bar. They exist only so a single
// agentctl instance's own /api/v1/system/metrics can be asked for them
// directly via ?metrics=, e.g. to measure whether tracker startup dominates
// instance creation before any optimization work is attempted.
const (
	MetricAgentctlGoroutines    = "agentctl_goroutines"
	MetricAgentctlGitPollMillis = "agentctl_git_poll_ms"
	MetricAgentctlMonitorMillis = "agentctl_monitor_poll_ms"
	MetricAgentctlCreateReadyMs = "agentctl_create_ready_ms"
)

type GlobalSettings struct {
	Metrics          []string `json:"metrics"`
	IntervalSeconds  int      `json:"interval_seconds"`
	BackendDiskPath  string   `json:"backend_disk_path"`
	CollectExecution bool     `json:"collect_execution"`
}

func DefaultSettings() GlobalSettings {
	return GlobalSettings{
		Metrics:          []string{MetricCPUPercent, MetricMemoryPercent, MetricDiskPercent},
		IntervalSeconds:  DefaultIntervalSeconds,
		BackendDiskPath:  "/",
		CollectExecution: false,
	}
}

func NormalizeSettings(in GlobalSettings) (GlobalSettings, error) {
	defaults := DefaultSettings()
	out := in
	if out.IntervalSeconds == 0 {
		out.IntervalSeconds = defaults.IntervalSeconds
	}
	if out.IntervalSeconds < MinIntervalSeconds || out.IntervalSeconds > MaxIntervalSeconds {
		return GlobalSettings{}, fmt.Errorf("%w: interval_seconds must be between %d and %d", ErrValidation, MinIntervalSeconds, MaxIntervalSeconds)
	}
	if out.BackendDiskPath == "" {
		out.BackendDiskPath = defaults.BackendDiskPath
	}
	if len(out.Metrics) == 0 {
		out.Metrics = defaults.Metrics
		return out, nil
	}

	seen := make(map[string]struct{}, len(out.Metrics))
	metrics := make([]string, 0, len(out.Metrics))
	for _, metric := range out.Metrics {
		if !isKnownMetric(metric) {
			return GlobalSettings{}, fmt.Errorf("%w: unknown metric %q", ErrValidation, metric)
		}
		if _, ok := seen[metric]; ok {
			continue
		}
		seen[metric] = struct{}{}
		metrics = append(metrics, metric)
	}
	if len(metrics) == 0 {
		return GlobalSettings{}, fmt.Errorf("%w: at least one metric is required", ErrValidation)
	}
	out.Metrics = metrics
	return out, nil
}

func isKnownMetric(metric string) bool {
	switch metric {
	case MetricCPUPercent, MetricMemoryPercent, MetricDiskPercent, MetricCPUTemp, MetricIOLoad:
		return true
	default:
		return false
	}
}
