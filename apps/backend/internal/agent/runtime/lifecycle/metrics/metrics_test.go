package metrics

import (
	"expvar"
	"testing"
)

func TestRestoreMetricFamiliesArePublished(t *testing.T) {
	for _, name := range []string{
		"agent_restore_attempts_total",
		"agent_restore_context_truncated_total",
		"agent_restore_recovery_required_total",
	} {
		if expvar.Get(name) == nil {
			t.Fatalf("expvar %q is not published", name)
		}
	}
}

func TestRestoreMetricsBoundLabels(t *testing.T) {
	RecordRestoreAttempt("not-a-real-outcome", "provider text", "provider-version-secret")
	RecordRestoreContextTruncated("provider-version-secret")
	RecordRecoveryRequired("arbitrary-consumer", "provider text")

	for _, name := range []string{
		"agent_restore_attempts_total",
		"agent_restore_context_truncated_total",
		"agent_restore_recovery_required_total",
	} {
		value := expvar.Get(name).String()
		if value == "{}" {
			t.Fatalf("expvar %q did not record an event", name)
		}
	}
}
