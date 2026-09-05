package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
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
