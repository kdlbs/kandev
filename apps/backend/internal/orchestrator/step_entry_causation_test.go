package orchestrator

import (
	"context"
	"testing"
)

// step_entry_causation_test.go covers AC-OFFICE-RUN-CAUSATION-001.25: every
// step-entry-triggered QueueRun call must carry the causing
// task_step_transitions ledger row's own id, both for the ledger-direct
// clear_decisions/queue_run action pair and for the marker-bearing
// queue_run_for_each_participant fan-out — reusing reviewLoopFixture from
// step_entry_dispatch_test.go so the same real engine + real sqlite ledger
// wiring is exercised.

// TestProcessOnEnter_QueueRunForEachParticipant_CarriesCausingStepTransitionID
// covers the marker-bearing path: before this fix, DispatchStepEntry's
// ledger-owned entryID never reached ExecuteMarkerBearingStepEntryAction, so
// every step-entry wake queued a run with no causing-transition identity and
// the office carrier resolver had nothing to root the causation chain on.
func TestProcessOnEnter_QueueRunForEachParticipant_CarriesCausingStepTransitionID(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count = %d, want 1", got)
	}
	if got := f.runQueue.calls[0].CausingStepTransitionID; got == "" {
		t.Fatalf("CausingStepTransitionID = %q, want non-empty ledger id for the step-entry wake", got)
	}
}

// TestProcessOnEnter_QueueRunForEachParticipant_CausingStepTransitionIDDiffersPerEntry
// mirrors AC-008.2's idempotency-key assertion for the causation identity:
// two distinct step entries (initial Review, then the rejection round's
// re-entry) must carry two distinct ledger ids, since a resolver keying off
// this value must be able to tell the two waves apart.
func TestProcessOnEnter_QueueRunForEachParticipant_CausingStepTransitionIDDiffersPerEntry(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Review -> Work, got none")
	}
	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if got := f.runQueue.callCount(); got != 2 {
		t.Fatalf("queued run count = %d, want 2", got)
	}

	first := f.runQueue.calls[0].CausingStepTransitionID
	second := f.runQueue.calls[1].CausingStepTransitionID
	if first == "" || second == "" {
		t.Fatalf("expected non-empty CausingStepTransitionID values, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("two distinct step entries must produce different ledger ids, both got %q", first)
	}
}
