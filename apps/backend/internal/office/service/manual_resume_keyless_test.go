package service_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// keylessCounterValue reads the current value of one
// office_run_dedup_keyless_total{reason,cause} label, or 0 if the label
// has never been reported. Callers snapshot before/after and assert the
// exact delta, so a call site that stops reporting (or a passing test
// that never actually reaches the report) can't hide behind a value some
// other test already set.
func keylessCounterValue(t *testing.T, reason string, cause runsservice.KeylessCause) int64 {
	t.Helper()
	v := expvar.Get("office_run_dedup_keyless_total")
	if v == nil {
		t.Fatalf("expvar map office_run_dedup_keyless_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_run_dedup_keyless_total is not a *expvar.Map")
	}
	iv := m.Get("reason=" + reason + ";cause=" + string(cause))
	if iv == nil {
		return 0
	}
	i, ok := iv.(*expvar.Int)
	if !ok {
		t.Fatalf("counter value for reason=%s;cause=%s is not *expvar.Int", reason, cause)
	}
	return i.Value()
}

// TestMarkAgentRunFailedFixed_ReportsKeylessRequeue pins
// requeueRunForTask's ReportKeylessEnqueue call (failure.go) — a manual
// resume has no prior occurrence row to key on, so it is keyless by
// design like the reactivity producers, but before this test that call
// site had zero coverage anywhere in the suite.
func TestMarkAgentRunFailedFixed_ReportsKeylessRequeue(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-manual-resume")
	taskID := "task-manual-resume-1"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-manual-resume")
	w := queueAndReadRun(t, svc, "agent-manual-resume", taskID)
	if err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	before := keylessCounterValue(t, service.RunReasonManualResumeAfterFailure, runsservice.KeylessCauseByDesign)

	if err := svc.MarkAgentRunFailedFixed(ctx, "user-1", w.ID); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	after := keylessCounterValue(t, service.RunReasonManualResumeAfterFailure, runsservice.KeylessCauseByDesign)
	if after != before+1 {
		t.Fatalf("office_run_dedup_keyless_total{%s,%s} = %d, want %d (one manual requeue)",
			service.RunReasonManualResumeAfterFailure, runsservice.KeylessCauseByDesign, after, before+1)
	}

	rows, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.AgentProfileID == "agent-manual-resume" && r.Reason == service.RunReasonManualResumeAfterFailure &&
			taskIDFromPayload(t, r.Payload) == taskID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a %s run queued for agent-manual-resume/%s, got %+v",
			service.RunReasonManualResumeAfterFailure, taskID, rows)
	}
}
