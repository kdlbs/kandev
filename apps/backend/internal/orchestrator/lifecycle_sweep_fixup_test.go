package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

type manualMoveMarkerClearBarrierRepo struct {
	sessionExecutorStore
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *manualMoveMarkerClearBarrierRepo) ClearManualMoveLifecycleMarkersIfCompleted(ctx context.Context, taskID string, completedAt time.Time) (bool, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return r.sessionExecutorStore.(manualMoveLifecycleMarkerCleaner).
		ClearManualMoveLifecycleMarkersIfCompleted(ctx, taskID, completedAt)
}

func TestRecoveryDoesNotClearPendingMarkerFromNewManualMove(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "manual-marker-race", "manual-marker-race-session", "destination-step")
	if err := repo.SetTaskMetadataKey(ctx, "manual-marker-race", models.MetaKeyManualMoveLifecyclePending,
		map[string]interface{}{"from_step_id": "old-source"}); err != nil {
		t.Fatalf("seed pending marker: %v", err)
	}
	if err := repo.SetTaskMetadataKey(ctx, "manual-marker-race", models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		t.Fatalf("seed completed marker: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{})
	release := make(chan struct{})
	guarded := &manualMoveMarkerClearBarrierRepo{
		sessionExecutorStore: repo,
		started:              make(chan struct{}),
		release:              release,
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	svc.repo = guarded

	recoveryDone := make(chan bool, 1)
	go func() { recoveryDone <- svc.recoverTaskLifecycleAttempt(ctx, "manual-marker-race") }()
	select {
	case <-guarded.started:
	case <-time.After(time.Second):
		t.Fatal("recovery did not reach the atomic marker clear")
	}

	// MoveTaskWithOptions performs this replacement in one task write. Model the
	// committed result directly so the test controls the exact interleaving.
	task, err := repo.GetTask(ctx, "manual-marker-race")
	if err != nil {
		t.Fatalf("load task for new move: %v", err)
	}
	task.WorkflowStepID = "new-destination-step"
	task.Metadata[models.MetaKeyManualMoveLifecyclePending] = map[string]interface{}{"from_step_id": "new-source"}
	delete(task.Metadata, models.MetaKeyManualMoveLifecycleCompleted)
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("commit newer manual move: %v", err)
	}
	close(release)

	if retry := <-recoveryDone; !retry {
		t.Fatal("recovery did not report remaining lifecycle work")
	}
	stored, err := repo.GetTask(ctx, "manual-marker-race")
	if err != nil {
		t.Fatalf("reload task after recovery: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
		t.Fatal("new manual move pending marker was cleared by old recovery")
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; completed {
		t.Fatal("new manual move unexpectedly retained old completion marker")
	}
}

func TestRecoveryRetriesWhenNewManualMoveAlreadyCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "manual-marker-completed-race", "manual-marker-completed-race-session", "destination-step")
	if err := repo.SetTaskMetadataKey(ctx, "manual-marker-completed-race", models.MetaKeyManualMoveLifecyclePending,
		map[string]interface{}{"from_step_id": "old-source"}); err != nil {
		t.Fatalf("seed pending marker: %v", err)
	}
	if err := repo.SetTaskMetadataKey(ctx, "manual-marker-completed-race", models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		t.Fatalf("seed completed marker: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{})
	release := make(chan struct{})
	guarded := &manualMoveMarkerClearBarrierRepo{
		sessionExecutorStore: repo,
		started:              make(chan struct{}),
		release:              release,
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	svc.repo = guarded

	recoveryDone := make(chan bool, 1)
	go func() { recoveryDone <- svc.recoverTaskLifecycleAttempt(ctx, "manual-marker-completed-race") }()
	select {
	case <-guarded.started:
	case <-time.After(time.Second):
		t.Fatal("recovery did not reach the atomic marker clear")
	}

	// A newer move can finish its lifecycle before the stale recovery clears
	// the old completion marker. Its completion marker has the same boolean
	// value, but its task update generation is different.
	task, err := repo.GetTask(ctx, "manual-marker-completed-race")
	if err != nil {
		t.Fatalf("load task for newer completion: %v", err)
	}
	delete(task.Metadata, models.MetaKeyManualMoveLifecyclePending)
	task.Metadata[models.MetaKeyManualMoveLifecycleCompleted] = true
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("commit newer completed move: %v", err)
	}
	close(release)

	if retry := <-recoveryDone; !retry {
		t.Fatal("recovery did not retain a newer completed marker for retry")
	}
	stored, err := repo.GetTask(ctx, "manual-marker-completed-race")
	if err != nil {
		t.Fatalf("reload task after recovery: %v", err)
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; !completed {
		t.Fatal("new manual move completion marker was cleared by old recovery")
	}
}

type failingManualMoveFeederPullReconciler struct{}

func (failingManualMoveFeederPullReconciler) ReconcileFeederPulls(context.Context, string, string) error {
	return errFeederReconcileTest
}

var errFeederReconcileTest = &feederReconcileTestError{}

type feederReconcileTestError struct{}

func (*feederReconcileTestError) Error() string { return "feeder reconciliation failed" }

func TestFailedManualMoveContinuationRetainsCompletionMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "manual-marker-error", "manual-marker-error-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "manual-marker-error", models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		t.Fatalf("seed completed marker: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetFeederPullReconciler(failingManualMoveFeederPullReconciler{})

	if retry := svc.recoverTaskLifecycleAttempt(ctx, "manual-marker-error"); !retry {
		t.Fatal("failed continuation did not request a retry")
	}
	stored, err := repo.GetTask(ctx, "manual-marker-error")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; !completed {
		t.Fatal("completion marker was cleared after feeder reconciliation failed")
	}
}

type watcherStateLifecycleRepo struct {
	sessionExecutorStore
	watcher  *watcher.Watcher
	observed chan bool
	release  <-chan struct{}
	once     sync.Once
}

func (r *watcherStateLifecycleRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	r.once.Do(func() {
		r.observed <- r.watcher.IsRunning()
	})
	<-r.release
	return r.sessionExecutorStore.(lifecycleTaskMetadataLister).ListTasksWithMetadataKey(ctx, key)
}

func TestStartupSweepReadsTokensAfterWatcherStarts(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "watcher-ordering", "watcher-ordering-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "watcher-ordering", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion marker: %v", err)
	}
	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	svc := NewService(DefaultServiceConfig(), eventBus, &mockAgentManager{}, newMockTaskRepo(), repo, nil, nil, nil, log)
	svc.SetTurnService(&repoTurnService{repo: repo})
	release := make(chan struct{})
	observed := make(chan bool, 1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		if svc.IsRunning() {
			if err := svc.Stop(); err != nil {
				t.Errorf("Stop: %v", err)
			}
		}
	})
	svc.repo = &watcherStateLifecycleRepo{
		sessionExecutorStore: repo,
		watcher:              svc.watcher,
		observed:             observed,
		release:              release,
	}
	startDone := make(chan error, 1)
	go func() { startDone <- svc.Start(ctx) }()

	select {
	case running := <-observed:
		if !running {
			close(release)
			<-startDone
			t.Fatal("startup lifecycle sweep read tokens before watcher subscriptions")
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Start and lifecycle sweep did not make progress")
	}
	close(release)
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return")
	}
}

type deadlineWorkContextRepo struct {
	sessionExecutorStore
	release      <-chan struct{}
	entered      chan struct{}
	observed     chan bool
	enteredOnce  sync.Once
	observedOnce sync.Once
}

func (r *deadlineWorkContextRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.enteredOnce.Do(func() { close(r.entered) })
	<-r.release
	r.observedOnce.Do(func() { r.observed <- ctx.Err() != nil })
	return r.sessionExecutorStore.GetTask(ctx, id)
}

func (r *deadlineWorkContextRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	return r.sessionExecutorStore.(lifecycleTaskMetadataLister).ListTasksWithMetadataKey(ctx, key)
}

func TestLifecycleSweepDeadlineDoesNotCancelAcceptedWorkContext(t *testing.T) {
	previousDeadline := lifecycleSweepOverallDeadline
	lifecycleSweepOverallDeadline = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepOverallDeadline = previousDeadline })

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "deadline-context", "deadline-context-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "deadline-context", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion marker: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	release := make(chan struct{})
	observed := make(chan bool, 1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	svc.repo = &deadlineWorkContextRepo{
		sessionExecutorStore: repo,
		release:              release,
		entered:              make(chan struct{}),
		observed:             observed,
	}
	sweepDone := make(chan struct{})
	go func() {
		svc.reconcileTaskLifecycleTokens(ctx)
		close(sweepDone)
	}()
	select {
	case <-svc.repo.(*deadlineWorkContextRepo).entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("worker was not admitted before the deadline")
	}
	select {
	case <-sweepDone:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("sweep did not return at its deadline")
	}
	close(release)
	select {
	case canceled := <-observed:
		if canceled {
			t.Fatal("deadline canceled a worker that was already admitted")
		}
	case <-time.After(time.Second):
		t.Fatal("admitted worker did not finish after deadline")
	}
}
