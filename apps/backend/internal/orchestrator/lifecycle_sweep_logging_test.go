package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// blockingLifecycleRepo blocks GetTask until release is closed, letting a
// test hold reconcileTaskLifecycleTokens open long enough to observe the
// stuck-sweep warning.
type blockingLifecycleRepo struct {
	sessionExecutorStore
	release <-chan struct{}
}

func (r *blockingLifecycleRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	<-r.release
	return r.sessionExecutorStore.GetTask(ctx, id)
}

func (r *blockingLifecycleRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	return r.sessionExecutorStore.(interface {
		ListTasksWithMetadataKey(context.Context, string) ([]*models.Task, error)
	}).ListTasksWithMetadataKey(ctx, key)
}

// dispatchTrackingLifecycleRepo behaves like blockingLifecycleRepo but also
// records which task IDs GetTask was called for, so a test can assert a job
// was never dispatched to a worker in the first place (as opposed to merely
// still being in flight).
type dispatchTrackingLifecycleRepo struct {
	sessionExecutorStore
	release <-chan struct{}
	mu      sync.Mutex
	seen    map[string]int
}

func (r *dispatchTrackingLifecycleRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	if r.seen == nil {
		r.seen = make(map[string]int)
	}
	r.seen[id]++
	r.mu.Unlock()
	<-r.release
	return r.sessionExecutorStore.GetTask(ctx, id)
}

func (r *dispatchTrackingLifecycleRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	return r.sessionExecutorStore.(interface {
		ListTasksWithMetadataKey(context.Context, string) ([]*models.Task, error)
	}).ListTasksWithMetadataKey(ctx, key)
}

func (r *dispatchTrackingLifecycleRepo) callCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[id]
}

// seedTaskAndSessionInWorkspace creates a task and session against an
// already-seeded workspace/workflow (e.g. from seedSession), for tests that
// need several tasks in the same sweep without recreating those rows.
func seedTaskAndSessionInWorkspace(t *testing.T, repo *sqliterepo.Repository, workspaceID, workflowID, taskID, sessionID, workflowStepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: workflowStepID,
		Title:          "Test Task",
		Description:    "Test",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task %s: %v", taskID, err)
	}

	session := &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create task session for %s: %v", taskID, err)
	}
}

func observedTestLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, logs
}

// observedTestLoggerWatching is observedTestLogger plus a channel that closes
// the first time a log entry with the given message is emitted, so a test can
// synchronize on the log itself instead of polling with time.Sleep.
func observedTestLoggerWatching(t *testing.T, message string) (*commonlogger.Logger, *observer.ObservedLogs, <-chan struct{}) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)

	seen := make(chan struct{})
	var once sync.Once
	hooked := zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		if entry.Message == message {
			once.Do(func() { close(seen) })
		}
		return nil
	})

	log, err := commonlogger.NewFromZap(zap.New(hooked))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, logs, seen
}

// TestReconcileTaskLifecycleTokensLogsStartAndFinish is the regression test
// for docs/plans/startup-listener-before-recovery/task-03-ordering-guard.md:
// a long startup sweep must be diagnosable from logs (task count, elapsed
// time) rather than requiring a goroutine dump.
//
// Expected pre-fix failure: reconcileTaskLifecycleTokens logs neither
// message today, so both FilterMessage results are empty.
func TestReconcileTaskLifecycleTokensLogsStartAndFinish(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "sweep-log-task", "sweep-log-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "sweep-log-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs := observedTestLogger(t)
	svc.logger = log

	svc.reconcileTaskLifecycleTokens(ctx)

	starts := logs.FilterMessage("startup lifecycle sweep starting").All()
	if len(starts) != 1 {
		t.Fatalf("start logs = %#v, want exactly one", starts)
	}
	if got := fmt.Sprint(starts[0].ContextMap()["task_count"]); got != "1" {
		t.Fatalf("start log task_count = %v, want 1", starts[0].ContextMap()["task_count"])
	}

	finishes := logs.FilterMessage("startup lifecycle sweep finished").All()
	if len(finishes) != 1 {
		t.Fatalf("finish logs = %#v, want exactly one", finishes)
	}
	if got := fmt.Sprint(finishes[0].ContextMap()["task_count"]); got != "1" {
		t.Fatalf("finish log task_count = %v, want 1", finishes[0].ContextMap()["task_count"])
	}
	if _, ok := finishes[0].ContextMap()["elapsed"]; !ok {
		t.Fatalf("finish log missing elapsed field: %#v", finishes[0].ContextMap())
	}
}

// TestReconcileTaskLifecycleTokensWarnsWhenStuck is the regression test for
// the "still recovering N tasks" log-only signal: a sweep still running
// after lifecycleSweepStuckWarningInterval must warn instead of staying
// silent, and must never cancel the in-flight recovery (context.WithoutCancel
// in task_operations.go means cancellation would not work anyway).
//
// Expected pre-fix failure: lifecycleSweepStuckWarningInterval does not
// exist yet, so this fails to compile.
func TestReconcileTaskLifecycleTokensWarnsWhenStuck(t *testing.T) {
	prevInterval := lifecycleSweepStuckWarningInterval
	lifecycleSweepStuckWarningInterval = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepStuckWarningInterval = prevInterval })

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "stuck-task", "stuck-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "stuck-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, _, warned := observedTestLoggerWatching(t, "startup lifecycle sweep still running")
	svc.logger = log

	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	select {
	case <-warned:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("stuck-sweep warning never logged")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not finish after release")
	}
}

// observedTestLoggerCounting is observedTestLoggerWatching plus a threshold:
// the returned channel closes once the message has been logged atLeast
// times, letting a test prove a warning repeats instead of firing once.
func observedTestLoggerCounting(t *testing.T, message string, atLeast int) (*commonlogger.Logger, <-chan struct{}) {
	t.Helper()
	core, _ := observer.New(zapcore.DebugLevel)

	var count atomic.Int32
	seen := make(chan struct{})
	var once sync.Once
	hooked := zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		if entry.Message == message && int(count.Add(1)) >= atLeast {
			once.Do(func() { close(seen) })
		}
		return nil
	})

	log, err := commonlogger.NewFromZap(zap.New(hooked))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, seen
}

// TestReconcileTaskLifecycleTokensWarnsRepeatedlyWhenStuck is the regression
// test for the stuck-sweep warning becoming a repeating ticker instead of a
// one-shot timer: a sweep still running after multiple
// lifecycleSweepStuckWarningInterval periods must warn every period, not
// just the first.
//
// Expected pre-fix failure: warnIfLifecycleSweepStuck uses a one-shot
// time.Timer, so only one "startup lifecycle sweep still running" entry is
// ever logged and this test times out waiting for a second.
func TestReconcileTaskLifecycleTokensWarnsRepeatedlyWhenStuck(t *testing.T) {
	prevInterval := lifecycleSweepStuckWarningInterval
	lifecycleSweepStuckWarningInterval = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepStuckWarningInterval = prevInterval })

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "stuck-task-repeat", "stuck-session-repeat", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "stuck-task-repeat", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, warnedTwice := observedTestLoggerCounting(t, "startup lifecycle sweep still running", 2)
	svc.logger = log

	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	select {
	case <-warnedTwice:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("stuck-sweep warning did not repeat")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not finish after release")
	}
}

// TestReconcileTaskLifecycleTokensDeadlineIncludesInFlightTaskIDs is the
// regression test for the sweep's overall-deadline exit branch: a worker
// still parked on a per-task resume when lifecycleSweepOverallDeadline fires
// must be named in the log, not just counted, and the function must return
// instead of waiting out the worker's own (much longer) interactive budget.
//
// Expected pre-fix failure: lifecycleSweepOverallDeadline does not exist, so
// this fails to compile.
func TestReconcileTaskLifecycleTokensDeadlineIncludesInFlightTaskIDs(t *testing.T) {
	prevDeadline := lifecycleSweepOverallDeadline
	lifecycleSweepOverallDeadline = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepOverallDeadline = prevDeadline })

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "deadline-task", "deadline-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "deadline-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs := observedTestLogger(t)
	svc.logger = log

	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("sweep did not return by its overall deadline while a worker was parked")
	}
	close(release)

	entries := logs.FilterMessage("startup lifecycle sweep exceeded its deadline; abandoning wait, in-flight tasks continue recovering in the background").All()
	if len(entries) != 1 {
		t.Fatalf("deadline-exceeded logs = %#v, want exactly one", entries)
	}
	ids, ok := entries[0].ContextMap()["in_flight_task_ids"].([]interface{})
	if !ok || len(ids) != 1 || ids[0] != "deadline-task" {
		t.Fatalf("in_flight_task_ids = %#v, want [\"deadline-task\"]", entries[0].ContextMap()["in_flight_task_ids"])
	}
}

// TestReconcileTaskLifecycleTokensDeadlineStopsUndispatchedWork is the
// regression test for the sweep's dispatch loop outliving its own overall
// deadline: only ctx.Done() stopped the dispatcher, so once every worker was
// parked on a per-task resume and the deadline fired, the function returned
// but the dispatcher kept the job channel open — any worker that later freed
// up (e.g. once its parked resume unblocked) would immediately pick up the
// next undispatched job, admitting new recovery work well past the advertised
// deadline instead of stopping at it.
//
// Five jobs with a worker pool of four guarantees one job is still sitting
// undispatched in the dispatcher's loop when the (short) deadline fires.
// jobs is a map, so which of the five ends up as that leftover job is
// randomized by Go's map iteration order — the test finds it by call count
// rather than assuming a fixed name.
//
// Expected pre-fix failure: after the parked workers are released, one of
// them picks up the previously-undispatched job and calls GetTask for it, so
// its call count goes from 0 to nonzero.
func TestReconcileTaskLifecycleTokensDeadlineStopsUndispatchedWork(t *testing.T) {
	prevDeadline := lifecycleSweepOverallDeadline
	lifecycleSweepOverallDeadline = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepOverallDeadline = prevDeadline })

	ctx := context.Background()
	repo := setupTestRepo(t)
	taskIDs := []string{"deadline-fan-1", "deadline-fan-2", "deadline-fan-3", "deadline-fan-4", "deadline-fan-5"}
	seedSession(t, repo, taskIDs[0], taskIDs[0]+"-session", "step1")
	for _, id := range taskIDs[1:] {
		seedTaskAndSessionInWorkspace(t, repo, "ws1", "wf1", id, id+"-session", "step1")
	}
	for _, id := range taskIDs {
		if err := repo.SetTaskMetadataKey(ctx, id, models.MetaKeyQueuePromotionPending, true); err != nil {
			t.Fatalf("seed promotion token for %s: %v", id, err)
		}
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	release := make(chan struct{})
	tracking := &dispatchTrackingLifecycleRepo{sessionExecutorStore: repo, release: release}
	svc.repo = tracking

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("sweep did not return by its overall deadline while workers were parked")
	}

	// reconcileTaskLifecycleTokens has already returned, so exactly
	// workerCount (4) of the 5 jobs should have reached a worker and be
	// blocked in GetTask; find the one that was never dispatched before
	// releasing the rest.
	var undispatched string
	var dispatchedBeforeDeadline int
	for _, id := range taskIDs {
		if tracking.callCount(id) == 0 {
			undispatched = id
		} else {
			dispatchedBeforeDeadline++
		}
	}
	if undispatched == "" || dispatchedBeforeDeadline != 4 {
		t.Fatalf("want exactly 4 of 5 jobs dispatched before the deadline and 1 left over, got %d dispatched, undispatched=%q", dispatchedBeforeDeadline, undispatched)
	}

	// Releasing the four parked workers now and giving them a moment to loop
	// back to a closed jobIDs channel proves the leftover job was never
	// handed out, rather than merely not yet handed out. There is no
	// production completion signal for this detached worker pool to
	// synchronize on instead (see stopLifecycleSweepAsync's own comment on
	// this being an intentionally un-joined background sweep).
	close(release)
	time.Sleep(100 * time.Millisecond)

	if got := tracking.callCount(undispatched); got != 0 {
		t.Fatalf("job %q was dispatched to a worker after the sweep's overall deadline had already fired: called %d times", undispatched, got)
	}
}

// TestReconcileTaskLifecycleTokensCancelIncludesInFlightTaskIDs is the
// regression test for the sweep's ctx-cancellation exit branch: cancelling
// the sweep's context while a worker is parked must return promptly and name
// the in-flight task, mirroring the deadline branch above.
//
// Expected pre-fix failure: the pre-fix "sweep cancelled" log line reported
// only a task count, so in_flight_task_ids is absent from its context.
func TestReconcileTaskLifecycleTokensCancelIncludesInFlightTaskIDs(t *testing.T) {
	prevInterval := lifecycleSweepStuckWarningInterval
	lifecycleSweepStuckWarningInterval = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepStuckWarningInterval = prevInterval })

	ctx, cancel := context.WithCancel(context.Background())
	repo := setupTestRepo(t)
	seedSession(t, repo, "cancel-task", "cancel-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "cancel-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs, stuck := observedTestLoggerWatching(t, "startup lifecycle sweep still running")
	svc.logger = log

	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	// Wait for proof the worker is actually in flight before cancelling, so
	// the cancellation log's in_flight_task_ids reflects the parked worker
	// instead of racing an empty snapshot.
	select {
	case <-stuck:
	case <-time.After(2 * time.Second):
		close(release)
		cancel()
		t.Fatal("worker never reported in-flight before cancellation")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("sweep did not return after ctx cancellation while a worker was parked")
	}
	close(release)

	entries := logs.FilterMessage("startup lifecycle sweep cancelled; abandoning wait, in-flight tasks continue recovering in the background").All()
	if len(entries) != 1 {
		t.Fatalf("cancelled logs = %#v, want exactly one", entries)
	}
	ids, ok := entries[0].ContextMap()["in_flight_task_ids"].([]interface{})
	if !ok || len(ids) != 1 || ids[0] != "cancel-task" {
		t.Fatalf("in_flight_task_ids = %#v, want [\"cancel-task\"]", entries[0].ContextMap()["in_flight_task_ids"])
	}
}
