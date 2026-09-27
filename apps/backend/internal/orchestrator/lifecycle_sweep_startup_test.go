package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type lifecycleTaskEventPublisher struct {
	eventBus eventbus.EventBus
	err      error
}

func (p *lifecycleTaskEventPublisher) PublishTaskUpdated(
	ctx context.Context,
	task *models.Task,
	_ ...string,
) {
	p.err = p.eventBus.Publish(ctx, events.TaskUpdated, eventbus.NewEvent(
		events.TaskUpdated,
		"lifecycle-test",
		map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"created_at":   task.CreatedAt,
			"updated_at":   task.UpdatedAt,
		},
	))
}

func (*lifecycleTaskEventPublisher) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {
}

func (*lifecycleTaskEventPublisher) PublishTaskActivityIfChanged(context.Context, string) {}

func (p *lifecycleTaskEventPublisher) publish(t *testing.T, ctx context.Context, task *models.Task) {
	t.Helper()
	p.PublishTaskUpdated(ctx, task)
	if p.err != nil {
		t.Fatalf("publish task.updated: %v", p.err)
	}
}

func loadProjectedTaskActivity(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID string) time.Time {
	t.Helper()
	summaries, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load task status summary: %v", err)
	}
	summary := summaries[taskID]
	if summary == nil || summary.LastActivityAt == nil {
		t.Fatalf("task summary = %#v, want projected activity", summary)
	}
	return *summary.LastActivityAt
}

func seedCompletedLifecycleTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, sessionID string) time.Time {
	t.Helper()
	seedSession(t, repo, taskID, sessionID, "step1")
	queueDB := sqlx.NewDb(repo.DB(), "sqlite3")
	if _, err := messagequeue.NewSQLiteRepository(queueDB, queueDB); err != nil {
		t.Fatalf("initialize queue schema: %v", err)
	}
	if err := repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		t.Fatalf("seed completed lifecycle marker: %v", err)
	}
	activityAt := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	if _, err := repo.DB().ExecContext(ctx,
		"UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?", activityAt, activityAt, taskID); err != nil {
		t.Fatalf("seed old task activity: %v", err)
	}
	return activityAt
}

func startLifecycleStatusProjector(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	eventBus eventbus.EventBus,
) {
	t.Helper()
	projectorCtx, cancel := context.WithCancel(ctx)
	projector := statussummary.NewProjector(statussummary.ProjectorConfig{
		Store:    repo,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "ws1", nil
		},
	})
	t.Cleanup(func() {
		cancel()
		projector.Close()
	})
	if err := projector.Start(projectorCtx); err != nil {
		t.Fatalf("start task status projector: %v", err)
	}
}

func assertReconstructedLifecycleActivity(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	taskID string,
	expected time.Time,
) {
	t.Helper()
	activityByTask, err := repo.LoadTaskLastActivity(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("reconstruct task activity after recovery: %v", err)
	}
	if got := activityByTask[taskID]; !got.Equal(expected) {
		t.Fatalf("reconstructed activity after marker cleanup = %s, want %s", got, expected)
	}
}

// TestRecoverTaskLifecycleTokenPreservesLastActivityDuringMarkerCleanup covers
// AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.9 through the real status projector.
func TestRecoverTaskLifecycleTokenPreservesLastActivityDuringMarkerCleanup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "completed-activity-task"
	const sessionID = "completed-activity-session"
	activityAt := seedCompletedLifecycleTask(t, ctx, repo, taskID, sessionID)
	eventBus := eventbus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	startLifecycleStatusProjector(t, ctx, repo, eventBus)

	publisher := &lifecycleTaskEventPublisher{eventBus: eventBus}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetTaskEventPublisher(publisher)
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{})
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load task before recovery: %v", err)
	}
	publisher.publish(t, ctx, task)
	if got := loadProjectedTaskActivity(t, ctx, repo, taskID); !got.Equal(activityAt) {
		t.Fatalf("projected activity before recovery = %s, want %s", got, activityAt)
	}

	svc.recoverTaskLifecycleToken(ctx, taskID)
	stored, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload task after recovery: %v", err)
	}
	if !stored.UpdatedAt.Equal(activityAt) {
		t.Fatalf("task updated_at after marker cleanup = %s, want unchanged %s", stored.UpdatedAt, activityAt)
	}
	if stored.State != task.State {
		t.Fatalf("task state after marker cleanup = %q, want unchanged %q", stored.State, task.State)
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; completed {
		t.Fatal("completed lifecycle marker was not cleared")
	}
	if got := loadProjectedTaskActivity(t, ctx, repo, taskID); !got.Equal(activityAt) {
		t.Fatalf("projected activity after marker cleanup = %s, want unchanged %s", got, activityAt)
	}
	assertReconstructedLifecycleActivity(t, ctx, repo, taskID, activityAt)

	stored.Title = "real task edit"
	if err := repo.UpdateTask(ctx, stored); err != nil {
		t.Fatalf("edit task after recovery: %v", err)
	}
	edited, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload edited task: %v", err)
	}
	publisher.publish(t, ctx, edited)
	if got := loadProjectedTaskActivity(t, ctx, repo, taskID); !got.After(activityAt) {
		t.Fatalf("projected activity after a genuine task edit = %s, want later than %s", got, activityAt)
	}
}

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
	done  chan struct{}
	once  sync.Once
}

func (r *countingFeederPullReconciler) ReconcileFeederPulls(context.Context, string, string) error {
	r.calls.Add(1)
	if r.done != nil {
		r.once.Do(func() { close(r.done) })
	}
	return nil
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
	publisher := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(publisher)

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
	if len(publisher.updatedTaskIDs) != 1 || publisher.updatedTaskIDs[0] != "completed-only-task" {
		t.Fatalf("task.updated publishes = %v, want one update for completed-only-task", publisher.updatedTaskIDs)
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
//
// It also covers the follow-on defect the raw-key fix introduced by itself:
// once the guard correctly detects the coexisting pending token, it must not
// simply leave both keys in place — that just swaps "replays forever" for
// "never converges," re-listing this task and re-running its continuation on
// every future startup. The stale pending token has to be cleared directly
// (not through recoverManualMoveLifecycle, which would replay the move), and
// the completed token cleared alongside it, so both converge in one pass.
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
	stored, err := repo.GetTask(ctx, "coexist-task")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; pending {
		t.Fatal("stale manual move lifecycle pending token was not cleared; sweep will never converge")
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; completed {
		t.Fatal("manual move lifecycle completion token was not cleared; sweep will never converge")
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

// TestLifecycleSweepStartRacingStopDoesNotMisuseWaitGroup is the regression
// test for lifecycleSweepWorkers.Add being published outside lifecycleSweepMu:
// startLifecycleSweepAsync used to unlock the mutex and only then call
// lifecycleSweepWorkers.Add(1), so a concurrent stopLifecycleSweepAsync could
// take the lock, read the (already-published) cancel func, and call
// lifecycleSweepWorkers.Wait() before Add(1) ran — the Go runtime's race
// detector treats an Add not ordered before a concurrent Wait as WaitGroup
// misuse, because nothing in the source establishes a happens-before edge
// between them. The fix moves Add(1) inside the same critical section as the
// cancel-func publish, matching dynamic_launch.go's
// launchDynamicSuccessorDetached. Run with `-race`; iterated because the
// unsynchronized window pre-fix is only a couple of instructions wide, so any
// single iteration has low odds of landing in it — repetition raises the
// odds of catching a reintroduced regression without claiming certainty.
//
// Expected pre-fix failure: `go test -race` reports a data race / WaitGroup
// misuse on lifecycleSweepWorkers (probabilistically, across repeated runs).
func TestLifecycleSweepStartRacingStopDoesNotMisuseWaitGroup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const iterations = 200
	for i := 0; i < iterations; i++ {
		svc.resetLifecycleSweepWorkers()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			svc.startLifecycleSweepAsync(ctx)
		}()
		go func() {
			defer wg.Done()
			svc.stopLifecycleSweepAsync()
		}()
		wg.Wait()
	}
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
	feederDone := make(chan struct{})
	svc.onManualMoveLifecycleStart = func() { starts.Add(1) }
	svc.SetFeederPullReconciler(&countingFeederPullReconciler{done: feederDone})

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

	select {
	case <-feederDone:
	case <-time.After(2 * time.Second):
		t.Fatal("manual move feeder continuation never completed")
	}
	stored, err := repo.GetTask(ctx, "concurrent-manual-move-task")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; pending {
		t.Fatal("manual move lifecycle pending token never cleared")
	}

	if got := starts.Load(); got != 1 {
		t.Fatalf("manual move step exit/enter side effects ran %d times concurrently, want exactly 1", got)
	}
}
