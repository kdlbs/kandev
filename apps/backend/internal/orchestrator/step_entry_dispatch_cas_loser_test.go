package orchestrator

// step_entry_dispatch_cas_loser_test.go covers two MEDIUM/P2 findings from
// this Build round's Review, both rooted in
// dispatchEngineOwnedOnEnterAction's !claimed branch:
//
//  1. It unconditionally returned "not failed", so a retry of a step-entry
//     whose clear_decisions had already recorded a terminal failure would
//     silently fall through to dispatching queue_run_for_each_participant —
//     the exact fall-through AC-C2's break exists to close for a fresh
//     failure, just reachable a second time through the CAS-loser path
//     instead.
//  2. It also returned "not failed" — indistinguishable from "already
//     done" — when the existing marker was still in_progress, i.e. a *live*
//     concurrent dispatch of the same step entry hadn't finished yet.
//     dispatchOnEnterActions only aborted on failed==true for
//     clear_decisions specifically, so the loser of that live claim would
//     skip ahead to queue_run_for_each_participant instead of abandoning
//     the entry — violating docs/specs/workflow-on-enter-action-dispatch/
//     spec.md's AC-F1 ("A dispatcher that loses a claim stops. It does not
//     skip ahead.") and risking the exact hazard AC-F1's rationale names:
//     enqueueing the fan-out before the winner's clear_decisions delete
//     (AC-A3) commits.
//
// See GetStepEntryMarkerState and its call site in
// dispatchEngineOwnedOnEnterAction for both.

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
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
	dispatchResult := f.svc.dispatchOnEnterActions(ctx, "t1", session, reviewStep, retryEntryID, false, false)
	if dispatchResult.hasAutoStart {
		t.Errorf("hasAutoStart = true, want false (Review declares no auto_start_agent)")
	}

	if got := f.runQueue.callCount(); got != 0 {
		t.Fatalf("queued run count after retry = %d, want still 0 (clear_decisions already failed terminally for this entry)", got)
	}
}

// blockingDecisionStore wraps fakeDecisionStore but parks ClearStepDecisions
// on a channel the test controls, so another goroutine can observe this
// entry's clear_decisions marker verifiably in_progress — a live claim, not
// a hypothetical race — before the test lets it proceed to completion.
type blockingDecisionStore struct {
	fakeDecisionStore
	proceed chan struct{}
}

func (f *blockingDecisionStore) ClearStepDecisions(ctx context.Context, taskID, stepID string) (int64, error) {
	<-f.proceed
	return f.fakeDecisionStore.ClearStepDecisions(ctx, taskID, stepID)
}

// TestProcessOnEnter_ConcurrentDispatchLosesLiveClaim_AbandonsEntryWithoutSkippingAhead
// drives two genuinely concurrent dispatches of the *same* step entry. The
// winner is parked mid-flight inside ClearStepDecisions, so its position-0
// marker is confirmed in_progress (polled, not assumed) before the second,
// would-be-loser dispatchOnEnterActions call races in on the same entryID.
// The loser must abandon the entry — no run queued, no WARNING/ERROR log
// (AC-F1: losing a claim is a normal outcome) — rather than skip ahead to
// queue_run_for_each_participant. Once the winner is released, it alone
// completes both actions.
func TestProcessOnEnter_ConcurrentDispatchLosesLiveClaim_AbandonsEntryWithoutSkippingAhead(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	store := &blockingDecisionStore{proceed: make(chan struct{})}
	f.svc.SetEngineDecisionStore(store)

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	f.svc.logger = log

	setSessionState(t, ctx, f.repo, "s1", models.TaskSessionStateRunning)
	onEnterDone := make(chan struct{})
	f.svc.onProcessOnEnterComplete = func() { close(onEnterDone) }

	session, err := f.repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	go f.svc.processOnTurnCompleteViaEngine(ctx, "t1", session)

	// entryID 1: the fresh per-test database's very first allocated
	// workflow_step_entries row (same reasoning as the sibling test above).
	const entryID = int64(1)
	const clearDecisionsPosition = 0

	deadline := time.Now().Add(2 * time.Second)
	for {
		state, _, found, stateErr := f.repo.GetStepEntryMarkerState(ctx, entryID, clearDecisionsPosition)
		if stateErr != nil {
			t.Fatalf("GetStepEntryMarkerState: %v", stateErr)
		}
		if found && state == stepentry.MarkerInProgress {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for position %d to become in_progress (found=%v state=%v)", clearDecisionsPosition, found, state)
		}
		time.Sleep(time.Millisecond)
	}

	sg, nameToID := buildWorkflowFromJSON(t, reviewLoopWorkflowJSON)
	reviewStep, ok := sg.steps[nameToID["Review"]]
	if !ok {
		t.Fatalf("Review step not found in rebuilt workflow")
	}

	// The concurrent, would-be-loser dispatch: same entry, same session,
	// racing while the winner above is still parked inside clear_decisions.
	dispatchResult := f.svc.dispatchOnEnterActions(ctx, "t1", session, reviewStep, entryID, false, false)
	if dispatchResult.hasAutoStart {
		t.Errorf("hasAutoStart = true, want false (Review declares no auto_start_agent)")
	}
	if got := f.runQueue.callCount(); got != 0 {
		t.Fatalf("queued run count after losing a live claim = %d, want 0 (AC-F1: loser must not skip ahead to queue_run_for_each_participant)", got)
	}
	for _, e := range logs.All() {
		if e.Level >= zapcore.WarnLevel {
			t.Errorf("unexpected %s log for a normal claim loss (AC-F1: not a fault): %s %+v", e.Level, e.Message, e.ContextMap())
		}
	}

	close(store.proceed)
	select {
	case <-onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for winner's on_enter processing to finish")
	}

	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count after winner finished = %d, want exactly 1 (winner alone dispatches queue_run_for_each_participant)", got)
	}
	store.mu.Lock()
	clearCalls := store.clearCalls
	store.mu.Unlock()
	if clearCalls != 1 {
		t.Errorf("clear_decisions calls = %d, want exactly 1 (loser must not have re-executed it)", clearCalls)
	}
}

// TestProcessOnEnter_RedispatchAfterEntryAlreadyDone_IsANoOp covers the third
// !claimed outcome GetStepEntryMarkerState can report — MarkerDone — which
// neither sibling test above exercises: the first test's retry sees
// MarkerFailed (a prior clear_decisions error), and the concurrent test's
// loser sees MarkerInProgress (a live peer). Here every position for the
// entry has already finished successfully via a normal, uncontested
// fireOnTurnComplete; a second, later dispatchOnEnterActions call for that
// same entryID (e.g. a duplicate trigger delivery, or any caller re-driving
// on_enter for a step entry the engine already fully processed) must fall
// through both positions as already-done and produce no new side effects,
// per the "MarkerDone: ... Safe, and required, to continue past it" branch
// in dispatchEngineOwnedOnEnterAction.
func TestProcessOnEnter_RedispatchAfterEntryAlreadyDone_IsANoOp(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count after the original dispatch = %d, want exactly 1", got)
	}
	f.decisions.mu.Lock()
	clearCallsAfterOriginal := f.decisions.clearCalls
	f.decisions.mu.Unlock()
	if clearCallsAfterOriginal != 1 {
		t.Fatalf("clear_decisions calls after the original dispatch = %d, want exactly 1", clearCallsAfterOriginal)
	}

	session, err := f.repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	sg, nameToID := buildWorkflowFromJSON(t, reviewLoopWorkflowJSON)
	reviewStep, ok := sg.steps[nameToID["Review"]]
	if !ok {
		t.Fatalf("Review step not found in rebuilt workflow")
	}

	// entryID 1: same fully-completed entry the fireOnTurnComplete call above
	// just processed; both of its on_enter positions are now MarkerDone.
	const entryID = int64(1)
	dispatchResult := f.svc.dispatchOnEnterActions(ctx, "t1", session, reviewStep, entryID, false, false)
	if dispatchResult.hasAutoStart {
		t.Errorf("hasAutoStart = true, want false (Review declares no auto_start_agent)")
	}

	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count after redispatching an already-done entry = %d, want still 1 (no re-execution)", got)
	}
	f.decisions.mu.Lock()
	clearCallsAfterRedispatch := f.decisions.clearCalls
	f.decisions.mu.Unlock()
	if clearCallsAfterRedispatch != 1 {
		t.Errorf("clear_decisions calls after redispatching an already-done entry = %d, want still 1 (no re-execution)", clearCallsAfterRedispatch)
	}
}
