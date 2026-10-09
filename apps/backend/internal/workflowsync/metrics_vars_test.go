package workflowsync

import (
	"expvar"
	"strconv"
	"testing"
)

func workflowSyncCounterValue(t *testing.T, label string) int64 {
	t.Helper()
	value := workflowSyncTransitionsTotal.Get(label)
	if value == nil {
		return 0
	}
	result, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		t.Fatalf("counter %q value is not an integer: %s", label, value.String())
	}
	return result
}

func TestWorkflowSyncTransitionCounter(t *testing.T) {
	if expvar.Get("workflow_sync_recovery_transitions_total") == nil {
		t.Fatal("workflow_sync_recovery_transitions_total is not published")
	}
	label := workflowSyncMetricLabel(
		"transition", "suspended", "provider", ProviderGitHub,
		"failure_class", "missing_resource", "retry_source", "",
	)
	before := workflowSyncCounterValue(t, label)
	incWorkflowSyncTransition("suspended", ProviderGitHub, "missing_resource", "")
	if delta := workflowSyncCounterValue(t, label) - before; delta != 1 {
		t.Fatalf("transition counter delta = %d, want 1", delta)
	}
}
