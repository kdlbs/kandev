package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeSettingsDefaults(t *testing.T) {
	got, err := NormalizeSettings(GlobalSettings{})
	if err != nil {
		t.Fatalf("NormalizeSettings returned error: %v", err)
	}

	if got.IntervalSeconds != DefaultIntervalSeconds {
		t.Fatalf("IntervalSeconds=%d, want %d", got.IntervalSeconds, DefaultIntervalSeconds)
	}
	if len(got.Metrics) != 3 {
		t.Fatalf("metrics len=%d, want 3", len(got.Metrics))
	}
	if got.CollectExecution {
		t.Fatal("CollectExecution should default to false")
	}
}

func TestNormalizeSettingsValidatesIntervalAndMetrics(t *testing.T) {
	_, err := NormalizeSettings(GlobalSettings{
		IntervalSeconds: 6 * 60,
		Metrics:         []string{MetricCPUPercent},
	})
	if err == nil {
		t.Fatal("expected error for interval above max")
	}

	got, err := NormalizeSettings(GlobalSettings{
		IntervalSeconds: 1,
		Metrics:         []string{MetricCPUPercent, MetricCPUPercent, MetricMemoryPercent},
	})
	if err != nil {
		t.Fatalf("NormalizeSettings returned error: %v", err)
	}
	if len(got.Metrics) != 2 {
		t.Fatalf("deduped metrics len=%d, want 2", len(got.Metrics))
	}

	_, err = NormalizeSettings(GlobalSettings{
		IntervalSeconds: 5,
		Metrics:         []string{"unknown"},
	})
	if err == nil {
		t.Fatal("expected error for unknown metric")
	}
}

// TestNormalizeSettingsRejectsAgentctlDiagnosticMetrics guards the design
// decision that keeps the per-instance agentctl diagnostic metrics
// (goroutine count, git poll latency, workspace monitor latency, create-to-ready time) out of persisted
// settings. If NormalizeSettings ever accepted one of these IDs it could be
// saved into GlobalSettings and would then reach the periodic broadcast and
// the status bar's METRIC_OPTIONS surface — a user-visible contract change
// this card deliberately does not make.
func TestNormalizeSettingsRejectsAgentctlDiagnosticMetrics(t *testing.T) {
	for _, id := range []string{MetricAgentctlGoroutines, MetricAgentctlGitPollMillis, MetricAgentctlMonitorMillis, MetricAgentctlCreateReadyMs} {
		t.Run(id, func(t *testing.T) {
			_, err := NormalizeSettings(GlobalSettings{
				IntervalSeconds: DefaultIntervalSeconds,
				Metrics:         []string{id},
			})
			if err == nil {
				t.Fatalf("expected NormalizeSettings to reject diagnostic metric %q", id)
			}
		})
	}
}

func TestCollectorResetClearsCPUBaseline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("cpu  1 0 0 9 0 0 0 0\n"), 0o600); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	collector := NewCollector()
	collector.procRoot = dir

	if _, err := collector.cpuPercent(); err != nil {
		t.Fatalf("cpuPercent baseline: %v", err)
	}
	if collector.prevCPU == nil {
		t.Fatal("expected CPU baseline to be stored")
	}

	collector.Reset()
	if collector.prevCPU != nil {
		t.Fatal("expected Reset to clear CPU baseline")
	}
	if !collector.lastCPUAt.IsZero() {
		t.Fatal("expected Reset to clear CPU timestamp")
	}
}

func TestCollectorCPUPercentResetsStaleBaseline(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	if err := os.WriteFile(statPath, []byte("cpu  1 0 0 9 0 0 0 0\n"), 0o600); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	collector := NewCollector()
	collector.procRoot = dir
	if value, err := collector.cpuPercent(); err != nil || value != 0 {
		t.Fatalf("cpuPercent baseline=(%v, %v), want 0 nil", value, err)
	}
	collector.lastCPUAt = time.Now().Add(-time.Duration(2*MaxIntervalSeconds+1) * time.Second)
	if err := os.WriteFile(statPath, []byte("cpu  10 0 0 10 0 0 0 0\n"), 0o600); err != nil {
		t.Fatalf("rewrite stat: %v", err)
	}
	if value, err := collector.cpuPercent(); err != nil || value != 0 {
		t.Fatalf("stale cpuPercent=(%v, %v), want 0 nil", value, err)
	}
}
