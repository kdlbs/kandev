package signalmetrics

import (
	"expvar"
	"strconv"
	"strings"
	"testing"
)

// readCounter walks the expvar map looking for a key that matches the
// supplied prefix. Returns 0 when no key matches. The prefix match keeps the
// assertion robust against process-wide test pollution (other tests in this
// package may push entries with different labels).
func readCounter(t *testing.T, m *expvar.Map, prefix string) int64 {
	t.Helper()
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if !strings.HasPrefix(kv.Key, prefix) {
			return
		}
		n, err := strconv.ParseInt(kv.Value.String(), 10, 64)
		if err != nil {
			t.Fatalf("counter %q value not int: %s", kv.Key, kv.Value.String())
		}
		total += n
	})
	return total
}

func TestMetricLabel(t *testing.T) {
	cases := []struct {
		name  string
		pairs []string
		want  string
	}{
		{"single_pair", []string{"source", "agent"}, "source=agent"},
		{"empty_agent_type", []string{"source", "agent", "agent_type", ""},
			"source=agent;agent_type="},
		{"odd_args_returns_empty", []string{"source"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metricLabel(tc.pairs...); got != tc.want {
				t.Errorf("metricLabel(%v) = %q, want %q", tc.pairs, got, tc.want)
			}
		})
	}
}

func TestRecordSignalReceived(t *testing.T) {
	label := metricLabel("source", "agent", "agent_type", "claude-metrics-test")

	before := readCounter(t, workflowStepSignalReceived, label)
	RecordSignalReceived("agent", "claude-metrics-test")
	after := readCounter(t, workflowStepSignalReceived, label)

	if after-before != 1 {
		t.Errorf("received counter delta = %d, want 1", after-before)
	}
}

func TestRecordSignalFallbackUsed(t *testing.T) {
	const agentType = "claude-fallback-test"
	label := metricLabel("agent_type", agentType)
	receivedLabel := metricLabel("source", "manual_fallback", "agent_type", agentType)

	before := readCounter(t, workflowStepSignalFallbackUsed, label)
	receivedBefore := readCounter(t, workflowStepSignalReceived, receivedLabel)
	RecordSignalFallbackUsed(agentType)
	after := readCounter(t, workflowStepSignalFallbackUsed, label)
	receivedAfter := readCounter(t, workflowStepSignalReceived, receivedLabel)

	if after-before != 1 {
		t.Errorf("fallback counter delta = %d, want 1", after-before)
	}
	if receivedAfter != receivedBefore {
		t.Errorf("received counter changed for fallback-only record: before=%d after=%d", receivedBefore, receivedAfter)
	}
}

func TestWorkflowStepSignalReceivedPublishedAtKnownName(t *testing.T) {
	if expvar.Get("workflow_step_completion_signal_received_total") == nil {
		t.Error("expvar \"workflow_step_completion_signal_received_total\" not published — /debug/vars consumers will miss it")
	}
	if expvar.Get("workflow_step_completion_signal_fallback_used_total") == nil {
		t.Error("expvar \"workflow_step_completion_signal_fallback_used_total\" not published — /debug/vars consumers will miss it")
	}
}
