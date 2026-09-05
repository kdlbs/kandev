package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestStartDoesNotBlockOnLifecycleSweep is the regression test for the
// ordering defect described in
// docs/specs/startup-listener-before-recovery/spec.md: reconcileTaskLifecycleTokens
// used to run synchronously inside Service.Start, before the watcher
// subscribed. A pending lifecycle token whose recovery blocks (here,
// deliberately, via blockingLifecycleRepo) therefore held Start — and with
// it every /api/v1/* route behind the bootstrap 503 handler — open
// indefinitely.
//
// Expected pre-fix failure: Start calls reconcileTaskLifecycleTokens
// synchronously before returning, so it blocks on blockingLifecycleRepo's
// release channel and the test times out.
func TestStartDoesNotBlockOnLifecycleSweep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "sweep-block-task", "sweep-block-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "sweep-block-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}

	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	cfg := DefaultServiceConfig()
	svc := NewService(cfg, eventBus, &mockAgentManager{}, newMockTaskRepo(), repo, nil, nil, nil, log)
	svc.SetTurnService(&repoTurnService{repo: repo})

	// Swap in the blocking wrapper only after construction: NewService's
	// internal executor/scheduler wiring needs the real repoStore, but the
	// startup sweep itself reads through s.repo (the narrower
	// sessionExecutorStore field), so this is the same seam
	// lifecycle_sweep_logging_test.go uses.
	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	startErr := make(chan error, 1)
	go func() { startErr <- svc.Start(ctx) }()

	select {
	case err := <-startErr:
		if err != nil {
			close(release)
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Start blocked on the startup lifecycle sweep instead of running it in the background")
	}

	close(release)
	t.Cleanup(func() {
		if err := svc.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
}

// countingFeederPullReconciler counts ReconcileFeederPulls calls without
// asserting on the arguments, for tests that only care how many times the
// sweep replayed a continuation.
type countingFeederPullReconciler struct {
	calls atomic.Int32
}

func (r *countingFeederPullReconciler) ReconcileFeederPulls(context.Context, string, string) {
	r.calls.Add(1)
}

// TestRecoverTaskLifecycleTokenClearsInertCompletedToken is the regression
// test for the non-convergence defect: a task carrying only
// MetaKeyManualMoveLifecycleCompleted (its pending token already cleared)
// must have its continuation replayed exactly once and the completed marker
// cleared, not be re-listed and re-replayed on every future startup sweep.
//
// Expected pre-fix failure: recoverTaskLifecycleAttempt's exit predicate
// counts manualMoveLifecycleCompleted as "still pending", so
// recoverTaskLifecycleToken retries maxAttempts times (3 continuation
// replays) and the completed token is never cleared.
func TestRecoverTaskLifecycleTokenClearsInertCompletedToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "completed-only-task", "completed-only-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "completed-only-task", models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		t.Fatalf("seed completed token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorder := &countingFeederPullReconciler{}
	svc.SetFeederPullReconciler(recorder)

	svc.recoverTaskLifecycleToken(ctx, "completed-only-task")

	if got := recorder.calls.Load(); got != 1 {
		t.Fatalf("feeder reconcile calls = %d, want exactly 1 (no retries on an inert token)", got)
	}
	stored, err := repo.GetTask(ctx, "completed-only-task")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; completed {
		t.Fatal("inert manual move lifecycle completion token was not cleared")
	}
}

// TestRecoverTaskLifecycleTokenDoesNotReplayCoexistingPendingAndCompletedTokens
// is the regression test for the crash-window defect: persistManualMoveLifecycleCompletion
// writes MetaKeyManualMoveLifecycleCompleted and clears
// MetaKeyManualMoveLifecyclePending in two separate calls, so a crash (or a
// failed remove) between them can leave both keys set on a move that already
// ran its step exit/enter side effects. manualMoveLifecyclePending is defined
// as pending && !completed, so it is always false while completed is set and
// cannot guard the completed-token clear against resurrecting the leftover
// pending token.
//
// Expected pre-fix failure: the completed marker is cleared on the first
// attempt because the guard checks the always-false manualMoveLifecyclePending
// helper instead of the raw key; the retry loop then treats the leftover raw
// pending token as a live move and a second attempt calls
// recoverManualMoveLifecycle, replaying the step exit/enter side effects for a
// lifecycle that already finished.
func TestRecoverTaskLifecycleTokenDoesNotReplayCoexistingPendingAndCompletedTokens(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "coexist-task", "coexist-session", "destination-step")
	task, err := repo.GetTask(ctx, "coexist-task")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata = map[string]interface{}{
		models.MetaKeyManualMoveLifecyclePending:   map[string]interface{}{"from_step_id": "source-step"},
		models.MetaKeyManualMoveLifecycleCompleted: true,
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist coexisting tokens: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{
		ID: "source-step", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}}},
	}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type:   wfmodels.OnEnterSetSessionMode,
			Config: map[string]interface{}{"mode": "destination"},
		}}},
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	var replays atomic.Int32
	svc.onManualMoveLifecycleStart = func() { replays.Add(1) }
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{})

	svc.recoverTaskLifecycleToken(ctx, "coexist-task")

	if got := replays.Load(); got != 0 {
		t.Fatalf("manual move lifecycle side effects replayed %d times, want 0 (move already completed)", got)
	}
}

// TestStartLifecycleSweepAsyncSkipsWhenServiceStopped is the regression test
// for the unsynchronized lifecycleSweepCancel access: Start sets s.running
// before it reaches startLifecycleSweepAsync, so a Stop racing in that window
// used to read a still-nil lifecycleSweepCancel, do nothing, and let Start go
// on to launch an uncancellable sweep after shutdown had already begun.
// Mirrors TestLaunchDynamicSuccessorDetached_SkipsWhenServiceStopped.
//
// Expected pre-fix failure: startLifecycleSweepAsync has no stopped check, so
// it always launches the sweep and there is no boolean return value to assert
// against (this test would not compile against the pre-fix signature).
func TestStartLifecycleSweepAsyncSkipsWhenServiceStopped(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Simulate a Stop that wins the race before Start ever calls
	// startLifecycleSweepAsync.
	svc.stopLifecycleSweepAsync()

	if svc.startLifecycleSweepAsync(ctx) {
		t.Fatal("a stopped service launched a startup lifecycle sweep it will never cancel")
	}
	if svc.lifecycleSweepCancel != nil {
		t.Fatal("a skipped sweep must not leave a cancel func for a later Stop to find")
	}

	svc.resetLifecycleSweepWorkers()
	if !svc.startLifecycleSweepAsync(ctx) {
		t.Fatal("a restarted service refused to launch the startup lifecycle sweep")
	}
	svc.stopLifecycleSweepAsync()
}

// TestLifecycleSweepConcurrentWithLiveTaskMovedRunsManualMoveSideEffectsOnce
// is the regression test for the spec's own concurrent-safety claim
// (docs/specs/startup-listener-before-recovery/spec.md, Desired-behaviour):
// running the startup sweep concurrently with live watcher/scheduler traffic
// is safe because of the per-task lock (queuedMoveLifecycleLocks) — "this
// must be confirmed by test, not by assertion." Runs the sweep's recovery
// path (recoverTaskLifecycleToken) concurrently with a live task.moved
// redelivery of the same admitted manual move (handleTaskMoved) and asserts
// the step exit/enter side effects run exactly once regardless of which path
// wins the lock.
func TestLifecycleSweepConcurrentWithLiveTaskMovedRunsManualMoveSideEffectsOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "concurrent-manual-move-task", "concurrent-manual-move-session", "source-step")
	task, err := repo.GetTask(ctx, "concurrent-manual-move-task")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "destination-step"
	task.WIPAdmitted = true
	task.Metadata = map[string]interface{}{
		models.MetaKeyManualMoveLifecyclePending: map[string]interface{}{"from_step_id": "source-step"},
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist admitted move: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{
		ID: "source-step", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}}},
	}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type:   wfmodels.OnEnterSetSessionMode,
			Config: map[string]interface{}{"mode": "destination"},
		}}},
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	var starts atomic.Int32
	svc.onManualMoveLifecycleStart = func() { starts.Add(1) }
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{})

	var wg sync.WaitGroup
	ready := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-ready
		svc.recoverTaskLifecycleToken(ctx, "concurrent-manual-move-task")
	}()
	go func() {
		defer wg.Done()
		<-ready
		svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
			TaskID: "concurrent-manual-move-task", SessionID: "concurrent-manual-move-session",
			FromStepID: "source-step", ToStepID: "destination-step", WIPAdmitted: true,
		})
	}()
	close(ready)
	wg.Wait()

	// handleTaskMoved's manual-move barrier dispatches to a detached goroutine
	// (fromStepAndTargetForTaskMoved), and the sweep's own inert-token GC
	// (recoverTaskLifecycleAttempt) may race to clear the completed marker
	// before this check runs, so the only durable convergence signal is the
	// pending token's absence — poll for that instead of assuming either
	// goroutine finished when it returns, and instead of requiring the
	// completed marker to still be present.
	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, err := repo.GetTask(ctx, "concurrent-manual-move-task")
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("manual move lifecycle pending token never cleared")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := starts.Load(); got != 1 {
		t.Fatalf("manual move step exit/enter side effects ran %d times concurrently, want exactly 1", got)
	}
}
