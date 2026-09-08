package service_test

import (
	"errors"
	"expvar"
	"strings"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// counterHasLabel reports whether the named expvar.Map has an entry whose
// key contains every given substring. Counters are process-global expvar
// state (no reset hook), so tests assert presence/growth rather than exact
// values that could collide with other tests in the same binary run.
func counterHasLabel(t *testing.T, mapName string, substrs ...string) bool {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %q not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a *expvar.Map", mapName)
	}
	found := false
	m.Do(func(kv expvar.KeyValue) {
		matches := true
		for _, s := range substrs {
			if !strings.Contains(kv.Key, s) {
				matches = false
				break
			}
		}
		if matches {
			found = true
		}
	})
	return found
}

func TestQueueOutcomeNone_IsZeroValue(t *testing.T) {
	if runsservice.QueueOutcomeNone != "" {
		t.Fatalf("QueueOutcomeNone = %q, want empty string", runsservice.QueueOutcomeNone)
	}
	var zero runsservice.QueueOutcome
	if zero != runsservice.QueueOutcomeNone {
		t.Fatalf("zero QueueOutcome %q != QueueOutcomeNone", zero)
	}
}

func TestReportWindowedDedup(t *testing.T) {
	reason := "test_windowed_" + t.Name()
	outcome := runsservice.ReportWindowedDedup(runsservice.QueueSourceRuns, reason, "some-key")
	if outcome != runsservice.QueueOutcomeDeduped {
		t.Fatalf("outcome = %q, want deduped", outcome)
	}
	if !counterHasLabel(t, "office_run_dedup_total", "reason="+reason, "kind=windowed", "queue=runs") {
		t.Fatal("expected office_run_dedup_total to carry a windowed/runs entry for this reason")
	}
}

func TestReportInsertResult_NilError(t *testing.T) {
	outcome, err := runsservice.ReportInsertResult(runsservice.QueueSourceRuns, "r", "k", "agent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != runsservice.QueueOutcomeQueued {
		t.Fatalf("outcome = %q, want queued", outcome)
	}
}

func TestReportInsertResult_UniqueViolation(t *testing.T) {
	reason := "test_durable_" + t.Name()
	violation := errors.New("UNIQUE constraint failed: runs.idempotency_key")
	outcome, err := runsservice.ReportInsertResult(runsservice.QueueSourceRuns, reason, "k", "agent-1", violation)
	if err != nil {
		t.Fatalf("expected nil error on classified conflict, got %v", err)
	}
	if outcome != runsservice.QueueOutcomeDeduped {
		t.Fatalf("outcome = %q, want deduped", outcome)
	}
	if !counterHasLabel(t, "office_run_dedup_total", "reason="+reason, "kind=durable", "queue=runs") {
		t.Fatal("expected office_run_dedup_total to carry a durable/runs entry for this reason")
	}
}

func TestReportInsertResult_OtherError_PassesThrough(t *testing.T) {
	reason := "test_other_" + t.Name()
	diskErr := errors.New("disk I/O error")
	outcome, err := runsservice.ReportInsertResult(runsservice.QueueSourceRuns, reason, "k", "agent-1", diskErr)
	if !errors.Is(err, diskErr) {
		t.Fatalf("expected the original error to pass through unchanged, got %v", err)
	}
	if outcome != runsservice.QueueOutcomeNone {
		t.Fatalf("outcome = %q, want none", outcome)
	}
	if counterHasLabel(t, "office_run_dedup_total", "reason="+reason) {
		t.Fatal("a non-conflict error must not move the dedup counter")
	}
}

func TestReportDurableDedup_Wakeup(t *testing.T) {
	reason := "test_wakeup_" + t.Name()
	outcome := runsservice.ReportDurableDedup(runsservice.QueueSourceWakeup, reason, "k", "agent-1")
	if outcome != runsservice.QueueOutcomeDeduped {
		t.Fatalf("outcome = %q, want deduped", outcome)
	}
	if !counterHasLabel(t, "office_run_dedup_total", "reason="+reason, "kind=durable", "queue=wakeup") {
		t.Fatal("expected office_run_dedup_total to carry a durable/wakeup entry for this reason")
	}
}

func TestReportKeylessEnqueue_CountsBothCauses(t *testing.T) {
	reasonUnresolved := "test_keyless_unresolved_" + t.Name()
	reasonByDesign := "test_keyless_by_design_" + t.Name()

	runsservice.ReportKeylessEnqueue(reasonUnresolved, runsservice.KeylessCauseUnresolved, "some_detail")
	runsservice.ReportKeylessEnqueue(reasonByDesign, runsservice.KeylessCauseByDesign, "")

	if !counterHasLabel(t, "office_run_dedup_keyless_total", "reason="+reasonUnresolved, "cause=unresolved") {
		t.Fatal("expected office_run_dedup_keyless_total to carry the unresolved entry")
	}
	if !counterHasLabel(t, "office_run_dedup_keyless_total", "reason="+reasonByDesign, "cause=by_design") {
		t.Fatal("expected office_run_dedup_keyless_total to carry the by_design entry")
	}
}
