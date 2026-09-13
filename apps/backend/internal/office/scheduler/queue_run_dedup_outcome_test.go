package scheduler

import (
	"context"
	"expvar"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// windowedDedupCounterValue reads the current value of one
// office_run_dedup_total{reason,kind=windowed,queue=runs} label, or 0 if the
// label has never been reported. Callers snapshot before/after and assert
// the exact delta.
func windowedDedupCounterValue(t *testing.T, reason string) int64 {
	t.Helper()
	v := expvar.Get("office_run_dedup_total")
	if v == nil {
		t.Fatalf("expvar map office_run_dedup_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_run_dedup_total is not a *expvar.Map")
	}
	iv := m.Get("reason=" + reason + ";kind=windowed;queue=runs")
	if iv == nil {
		return 0
	}
	i, ok := iv.(*expvar.Int)
	if !ok {
		t.Fatalf("counter value for reason=%s;kind=windowed;queue=runs is not *expvar.Int", reason)
	}
	return i.Value()
}

// TestSchedulerService_QueueRun_ReportsWindowedDedupOutcome closes a
// phantom-green gap on the office-owned queue implementation the design
// names as least observable (run-dedup-generation-02.md#test-strategy):
// no test asserted SchedulerService.QueueRun actually calls
// ReportWindowedDedup, so deleting that call would leave every existing
// test green while AC-OFFICE-RUN-DEDUP-004.1 silently went unmet on the
// path a reassignment travels.
func TestSchedulerService_QueueRun_ReportsWindowedDedupOutcome(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-queue-run-outcome")
	ctx := context.Background()

	key := "idem-key-scheduler-outcome"
	first, err := ss.QueueRun(ctx, "agent-queue-run-outcome", RunReasonTaskAssigned, "{}", key)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %v, want QueueOutcomeQueued", first)
	}

	before := windowedDedupCounterValue(t, RunReasonTaskAssigned)

	second, err := ss.QueueRun(ctx, "agent-queue-run-outcome", RunReasonTaskAssigned, "{}", key)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if second != runsservice.QueueOutcomeDeduped {
		t.Fatalf("second outcome = %v, want QueueOutcomeDeduped", second)
	}

	after := windowedDedupCounterValue(t, RunReasonTaskAssigned)
	if after != before+1 {
		t.Fatalf("office_run_dedup_total{%s,windowed,runs} = %d, want %d", RunReasonTaskAssigned, after, before+1)
	}
}
