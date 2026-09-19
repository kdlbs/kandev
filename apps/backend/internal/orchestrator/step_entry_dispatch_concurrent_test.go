package orchestrator

// step_entry_dispatch_concurrent_test.go covers the HIGH duplicate-entry
// finding from this Build round's Review: two independent event sources
// (agent turn completion, agent-exit, the step_complete_kandev signal, user
// cancellation) can each call processOnTurnCompleteViaEngine for the same
// session. Without serialization, two concurrent calls could both read the
// same pre-transition step, both call applyEngineTransition, and each
// allocate its own workflow_step_entries row for what should be a single
// logical transition — double-dispatching that entry's on_enter actions
// (clear_decisions, queue_run_for_each_participant). See
// acquireTurnCompletionCriticalSection and turnCompletionLocks' field
// comment in service.go for the fix.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestProcessOnTurnComplete_ConcurrentDuplicateCalls_OnlyOneTransitionApplies
// fires two concurrent processOnTurnCompleteViaEngine calls for the same
// session, both observing the same pre-transition step (Work). Exactly one
// must win: apply the Work -> Review transition and dispatch Review's
// on_enter actions exactly once. The other must recognize (after losing the
// race for the per-session lock and re-reading fresh task state) that
// another caller already handled this turn, and return false without
// re-evaluating on_turn_complete against Review's own declaration — which
// would otherwise spuriously fire Review -> Work for a turn nobody in
// Review actually completed.
func TestProcessOnTurnComplete_ConcurrentDuplicateCalls_OnlyOneTransitionApplies(t *testing.T) {
	ctx := context.Background()
	f := newReviewLoopFixture(t)
	setSessionState(t, ctx, f.repo, "s1", models.TaskSessionStateRunning)

	onEnterDone := make(chan struct{}, 2)
	f.svc.onProcessOnEnterComplete = func() { onEnterDone <- struct{}{} }

	session, err := f.repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	const callers = 2
	results := make([]bool, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = f.svc.processOnTurnCompleteViaEngine(ctx, "t1", session)
		}(i)
	}
	close(start)
	wg.Wait()

	select {
	case <-onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for on_enter processing")
	}

	transitionedCount := 0
	for _, r := range results {
		if r {
			transitionedCount++
		}
	}
	if transitionedCount != 1 {
		t.Fatalf("transitioned count = %d, want exactly 1 of %d concurrent calls", transitionedCount, callers)
	}

	assertStepByName(t, ctx, f.repo, "s1", "Review", f.nameToID)

	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count = %d, want exactly 1 (no double-dispatch)", got)
	}

	f.decisions.mu.Lock()
	clearCalls := f.decisions.clearCalls
	f.decisions.mu.Unlock()
	if clearCalls != 1 {
		t.Fatalf("clear_decisions calls = %d, want exactly 1 (no double-dispatch)", clearCalls)
	}
}
