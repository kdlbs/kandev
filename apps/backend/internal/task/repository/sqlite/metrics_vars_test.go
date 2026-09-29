package sqlite

import (
	"expvar"
	"strconv"
	"testing"
	"time"
)

func TestMessagePayloadHydrationMetricsPublished(t *testing.T) {
	for _, name := range []string{
		"task_message_payload_hydrations_total",
		"task_message_payload_hydration_latency_ms",
	} {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q is not published", name)
		}
	}
}

func metricMapValue(t *testing.T, metric *expvar.Map, key string) int64 {
	t.Helper()
	value := metric.Get(key)
	if value == nil {
		return 0
	}
	got, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		t.Fatalf("metric %q value %q is not an integer: %v", key, value.String(), err)
	}
	return got
}

func TestRecordMessagePayloadHydrationUsesBoundedOutcomeAndLatencyLabels(t *testing.T) {
	successBefore := metricMapValue(t, messagePayloadHydrationsTotal, "outcome=success")
	bucketBefore := metricMapValue(t, messagePayloadHydrationMS, "outcome=success;le=50")
	recordMessagePayloadHydration(25*time.Millisecond, nil)
	if got := metricMapValue(t, messagePayloadHydrationsTotal, "outcome=success") - successBefore; got != 1 {
		t.Fatalf("success hydration delta = %d, want 1", got)
	}
	if got := metricMapValue(t, messagePayloadHydrationMS, "outcome=success;le=50") - bucketBefore; got != 1 {
		t.Fatalf("50ms hydration bucket delta = %d, want 1", got)
	}
}
