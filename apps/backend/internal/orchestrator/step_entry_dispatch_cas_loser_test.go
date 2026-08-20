package orchestrator

// step_entry_dispatch_cas_loser_test.go covers the MEDIUM finding from this
// Build round's Review: dispatchEngineOwnedOnEnterAction's !claimed branch
// unconditionally returned "not failed", so a retry of a step-entry whose
// clear_decisions had already recorded a terminal failure would silently
// fall through to dispatching queue_run_for_each_participant — the exact
// fall-through AC-C2's break exists to close for a fresh failure, just
// reachable a second time through the CAS-loser path instead. See
// GetStepEntryMarkerState and its call site in dispatchEngineOwnedOnEnterAction.

import (
	"context"
	"testing"
)

// TestProcessOnEnter_ClearDecisionsPriorFailure_BlocksRetryFromDispatchingSubsequentActions
// drives a step entry's on_enter dispatch twice: once with a failing
// decisions store (mirroring AC-C2's failure test, which records the
// clear_decisions marker as terminally failed for this entry), then again
// with a passing store — modelling a retry after whatever caused the first
// failure was fixed, but for the *same* step entry. The retry must still
// see clear_decisions as failed (from the stored marker state, since its
// CAS claim is lost to the earlier row) and must not proceed to dispatch
// queue_run_for_each_participant.
func TestProcessOnEnter_ClearDecisionsPriorFailure_BlocksRetryFromDispatchingSubsequentActions(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)
	f.svc.SetEngineDecisionStore(&failingDecisionStore{})

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if got := f.runQueue.callCount(); got != 0 {
		t.Fatalf("queued run count after clear_decisions failure = %d, want 0 (AC-C2)", got)
	}

	// Swap in a passing decisions store, as if the underlying problem that
	// caused the first clear_decisions attempt to fail were now resolved.
	f.svc.SetEngineDecisionStore(&fakeDecisionStore{})

	session, err := f.repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	sg, nameToID := buildWorkflowFromJSON(t, reviewLoopWorkflowJSON)
	reviewStep, ok := sg.steps[nameToID["Review"]]
	if !ok {
		t.Fatalf("Review step not found in rebuilt workflow")
	}

	// entryID 1: the fresh per-test database's very first allocated
	// workflow_step_entries row, created by fireOnTurnComplete's Work ->
	// Review transition above (no other entry allocation happens before
	// it in this fixture).
	const retryEntryID = int64(1)
	hasAutoStart := f.svc.dispatchOnEnterActions(ctx, "t1", session, reviewStep, retryEntryID, false, false)
	if hasAutoStart {
		t.Errorf("hasAutoStart = true, want false (Review declares no auto_start_agent)")
	}

	if got := f.runQueue.callCount(); got != 0 {
		t.Fatalf("queued run count after retry = %d, want still 0 (clear_decisions already failed terminally for this entry)", got)
	}
}
