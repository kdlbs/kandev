package metrics

import "testing"

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
