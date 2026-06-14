package metrics

import (
	"os"
	"path/filepath"
	"testing"
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
}
