package service_test

import (
	"errors"
	"expvar"
	"strings"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// counterValue returns the current value of one key in a *expvar.Map, or 0
// if the key has never been touched. Used instead of counterHasLabel's
// presence check because office_assignment_rate_limit_total's label set is
// the fixed three-value reason (no per-test-unique label to key off), so
// these tests read the delta across a call instead.
func counterValue(t *testing.T, mapName, key string) int64 {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %q not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a *expvar.Map", mapName)
	}
	var got int64
	m.Do(func(kv expvar.KeyValue) {
		if kv.Key != key {
			return
		}
		if iv, ok := kv.Value.(*expvar.Int); ok {
			got = iv.Value()
		}
	})
	return got
}

const assignmentRateLimitMap = "office_assignment_rate_limit_total"

// TestReportAssignmentRateLimitRefused covers AC-OFFICE-ASSIGN-RATE-003.1's
// allowance_exhausted counter and the new QueueOutcomeRateLimited value.
func TestReportAssignmentRateLimitRefused(t *testing.T) {
	before := counterValue(t, assignmentRateLimitMap, "reason=allowance_exhausted")

	outcome := runsservice.ReportAssignmentRateLimitRefused("task-1", "agent-assignee", "agent-actor", 5, 5)

	if outcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("outcome = %q, want rate_limited", outcome)
	}
	after := counterValue(t, assignmentRateLimitMap, "reason=allowance_exhausted")
	if after != before+1 {
		t.Fatalf("counter = %d, want %d", after, before+1)
	}
}

// TestReportAssignmentRateLimitCountReadFailed covers
// AC-OFFICE-ASSIGN-RATE-002.2's degraded-admission counter: the wake is
// still admitted, so this function has no return value to assert — only
// the counter move.
func TestReportAssignmentRateLimitCountReadFailed(t *testing.T) {
	before := counterValue(t, assignmentRateLimitMap, "reason=count_read_failed")

	runsservice.ReportAssignmentRateLimitCountReadFailed("task-1", "agent-assignee", "agent-actor", 5, errors.New("db unavailable"))

	after := counterValue(t, assignmentRateLimitMap, "reason=count_read_failed")
	if after != before+1 {
		t.Fatalf("counter = %d, want %d", after, before+1)
	}
}

// TestReportAssignmentRateLimitUnattributed covers
// AC-OFFICE-ASSIGN-RATE-002.3's degraded-admission counter for an
// in-scope wake whose task could not be determined.
func TestReportAssignmentRateLimitUnattributed(t *testing.T) {
	before := counterValue(t, assignmentRateLimitMap, "reason=task_unattributed")

	runsservice.ReportAssignmentRateLimitUnattributed("agent-actor", 5)

	after := counterValue(t, assignmentRateLimitMap, "reason=task_unattributed")
	if after != before+1 {
		t.Fatalf("counter = %d, want %d", after, before+1)
	}
}

// TestReportAssignmentRateLimitUnattributed_EmptyActorID covers the same
// path with no acting agent identifier, which the function must accept
// without panicking (the conditional log field append).
func TestReportAssignmentRateLimitUnattributed_EmptyActorID(t *testing.T) {
	before := counterValue(t, assignmentRateLimitMap, "reason=task_unattributed")

	runsservice.ReportAssignmentRateLimitUnattributed("", 5)

	after := counterValue(t, assignmentRateLimitMap, "reason=task_unattributed")
	if after != before+1 {
		t.Fatalf("counter = %d, want %d", after, before+1)
	}
}

// TestAssignmentRateLimitCounter_LabelHasOnlyReason covers
// AC-OFFICE-ASSIGN-RATE-003.2: the counter's key must carry only the
// closed reason label, never a task, agent, or run identifier.
func TestAssignmentRateLimitCounter_LabelHasOnlyReason(t *testing.T) {
	runsservice.ReportAssignmentRateLimitRefused("task-should-not-appear", "assignee-should-not-appear", "actor-should-not-appear", 5, 5)

	v := expvar.Get(assignmentRateLimitMap)
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a *expvar.Map", assignmentRateLimitMap)
	}
	m.Do(func(kv expvar.KeyValue) {
		if !strings.Contains(kv.Key, "allowance_exhausted") {
			return
		}
		if kv.Key != "reason=allowance_exhausted" {
			t.Fatalf("key = %q, want exactly \"reason=allowance_exhausted\" with no identifier labels", kv.Key)
		}
	})
}
