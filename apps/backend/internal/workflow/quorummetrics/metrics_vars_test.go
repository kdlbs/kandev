package quorummetrics

import (
	"expvar"
	"strconv"
	"testing"
)

func readCounter(t *testing.T, m *expvar.Map, key string) int64 {
	t.Helper()
	v := m.Get(key)
	if v == nil {
		return 0
	}
	n, err := strconv.ParseInt(v.String(), 10, 64)
	if err != nil {
		t.Fatalf("counter %q value not int: %s", key, v.String())
	}
	return n
}

func TestRecordGuardNotFired(t *testing.T) {
	before := readCounter(t, workflowQuorumGuardNotFired, "threshold_not_met")
	RecordGuardNotFired("threshold_not_met")
	after := readCounter(t, workflowQuorumGuardNotFired, "threshold_not_met")

	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestRecordGuardNotFired_SessionUnresolvableSharesTheSameCounter(t *testing.T) {
	before := readCounter(t, workflowQuorumGuardNotFired, "session_unresolvable")
	RecordGuardNotFired("session_unresolvable")
	after := readCounter(t, workflowQuorumGuardNotFired, "session_unresolvable")

	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestWorkflowQuorumGuardNotFiredPublishedAtKnownName(t *testing.T) {
	if expvar.Get("workflow_quorum_guard_not_fired_total") == nil {
		t.Error("expvar \"workflow_quorum_guard_not_fired_total\" not published — /debug/vars consumers will miss it")
	}
}
