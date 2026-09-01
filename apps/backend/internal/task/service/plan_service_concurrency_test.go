package service

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func waitForPlanLockWaiters(t *testing.T, locks *planLockTable, taskID string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		locks.mu.Lock()
		entry := locks.entries[taskID]
		got := 0
		if entry != nil {
			got = entry.waiters
		}
		locks.mu.Unlock()
		if got >= want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("task %q did not register %d lock holders or waiters", taskID, want)
}

// newFlakyPlanService builds a PlanService backed by the given underlying
// repository but wrapped so every GetTaskPlan call from callCount onward
// (1-based) fails. Callers seed data through repo directly (bypassing the
// flaky wrapper) before constructing this service.
func newFlakyPlanService(t *testing.T, repo *sqliterepo.Repository, eventBus *MockEventBus, failFromNth int) *PlanService {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	flaky := &flakyPlanReadRepo{Repository: repo, failFromNth: failFromNth}
	return NewPlanService(flaky, eventBus, log)
}

// flakyPlanReadRepo makes GetTaskPlan fail from its callCount'th invocation
// onward (1-based), simulating a transient read failure (e.g. SQLITE_BUSY)
// at a precise point in PlanService's read-then-write sequence.
type flakyPlanReadRepo struct {
	*sqliterepo.Repository
	callCount   int
	failFromNth int
}

func (r *flakyPlanReadRepo) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	r.callCount++
	if r.failFromNth > 0 && r.callCount >= r.failFromNth {
		return nil, errors.New("simulated read failure")
	}
	return r.Repository.GetTaskPlan(ctx, taskID)
}

// gatedWriteRepo blocks inside its first WritePlanRevision call until
// released, letting a test force a specific interleaving of two concurrent
// callers that share this same repository (and, critically, the same
// PlanService instance - and therefore the same per-task lock table).
// Gating is via an atomic CAS rather than sync.Once: Once.Do blocks *every*
// concurrent caller until the first call's function returns, which would
// make an unrelated-task write wait on this gate too and defeat the
// cross-task no-wait test. Only the goroutine that wins the CAS blocks;
// every other caller (concurrent or later) passes straight through.
type gatedWriteRepo struct {
	*sqliterepo.Repository
	writeStarted chan struct{}
	proceed      chan struct{}
	gated        int32
}

func (r *gatedWriteRepo) WritePlanRevision(ctx context.Context, head *models.TaskPlan, rev *models.TaskPlanRevision, coalesceLatestID *string, preserveTitle, preserveCreatedBy bool) error {
	if atomic.CompareAndSwapInt32(&r.gated, 0, 1) {
		close(r.writeStarted)
		<-r.proceed
	}
	return r.Repository.WritePlanRevision(ctx, head, rev, coalesceLatestID, preserveTitle, preserveCreatedBy)
}

// gatedDeleteRepo blocks inside its first DeleteTaskPlan call until
// released; see gatedWriteRepo for why gating uses an atomic CAS rather
// than sync.Once.
type gatedDeleteRepo struct {
	*sqliterepo.Repository
	deleteStarted chan struct{}
	proceed       chan struct{}
	gated         int32
}

func (r *gatedDeleteRepo) DeleteTaskPlan(ctx context.Context, taskID string) error {
	if atomic.CompareAndSwapInt32(&r.gated, 0, 1) {
		close(r.deleteStarted)
		<-r.proceed
	}
	return r.Repository.DeleteTaskPlan(ctx, taskID)
}

// newGuardedRelease returns a close-once function for a gated test's release
// channel, registered via t.Cleanup. Without this, a t.Fatalf firing between
// the gate opening and the test's intended release point exits the test
// goroutine (via runtime.Goexit) before close() runs, leaving the gated
// goroutine parked on <-r.proceed forever - holding the per-task lock and a
// live DB connection for the rest of the test binary's run.
func newGuardedRelease(t *testing.T, ch chan struct{}) func() {
	t.Helper()
	var once sync.Once
	release := func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release)
	return release
}

// TestConcurrentUpdatePlanForSameTaskIsMutuallyExclusive forces a specific
// interleaving: writer 1 is gated inside its write transaction (already
// holding the per-task lock) while writer 2 attempts a concurrent update on
// the same task. Writer 2 must not observe any result until writer 1's write
// releases the lock (AC-002, the core TOCTOU race this card exists to close).
func TestConcurrentUpdatePlanForSameTaskIsMutuallyExclusive(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-gate")

	// Seed directly through the underlying repo (not the gated wrapper below):
	// the gate fires on its *first* WritePlanRevision call, and this seed must
	// not be that call.
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-gate", Title: "Plan", Content: "v0", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-gate", Content: "from-first", AuthorKind: "agent", AuthorName: "A",
		})
		firstDone <- err
	}()
	<-writeStarted // writer 1 now holds the per-task lock, blocked inside its write tx

	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-gate", Content: "from-second", AuthorKind: "agent", AuthorName: "B",
		})
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second UpdatePlan returned (err=%v) before the gated first write released the per-task lock", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("first UpdatePlan: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second UpdatePlan: %v", err)
	}

	head, err := svc.GetPlan(ctx, "task-gate")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if head.Content != "from-second" {
		t.Errorf("expected HEAD to reflect the second (later, unblocked-after) write, got %q", head.Content)
	}

	list, err := svc.ListRevisions(ctx, "task-gate")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 revisions (2 distinct-author writes; the seed used CreateTaskPlan directly, with no revision row), got %d", len(list))
	}
}

// TestConcurrentWritesToDifferentTasksDoNotBlockEachOther proves the
// per-task lock table does not serialize unrelated tasks: a writer gated
// inside task A's write transaction must not prevent a write to task B from
// completing.
func TestConcurrentWritesToDifferentTasksDoNotBlockEachOther(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-a")
	seedTask(t, ctx, repo, "task-b")

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	aDone := make(chan error, 1)
	go func() {
		_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-a", Content: "a1"})
		aDone <- err
	}()
	<-writeStarted

	bDone := make(chan error, 1)
	go func() {
		_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-b", Content: "b1"})
		bDone <- err
	}()

	select {
	case err := <-bDone:
		if err != nil {
			t.Fatalf("CreatePlan for task-b: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write to an unrelated task (task-b) blocked behind task-a's held lock")
	}

	release()
	if err := <-aDone; err != nil {
		t.Fatalf("CreatePlan for task-a: %v", err)
	}
}

// TestDeletePlanHoldsLockAcrossConcurrentCreate proves DeletePlan's lock
// scope covers its whole existence-check-then-delete critical section
// (F37): a concurrent CreatePlan on the same task must not interleave
// between the check and the delete.
func TestDeletePlanHoldsLockAcrossConcurrentCreate(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-del-create")

	deleteStarted := make(chan struct{})
	releaseDelete := make(chan struct{})
	release := newGuardedRelease(t, releaseDelete)
	gated := &gatedDeleteRepo{Repository: repo, deleteStarted: deleteStarted, proceed: releaseDelete}
	svc := NewPlanService(gated, eventBus, log)

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-del-create", Content: "v1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- svc.DeletePlan(ctx, "task-del-create")
	}()
	<-deleteStarted

	createDone := make(chan struct {
		result PlanWriteResult
		err    error
	}, 1)
	go func() {
		result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-del-create", Content: "recreated"})
		createDone <- struct {
			result PlanWriteResult
			err    error
		}{result, err}
	}()

	select {
	case out := <-createDone:
		t.Fatalf("concurrent CreatePlan finished (err=%v) before the gated DeletePlan released its lock", out.err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	if err := <-deleteDone; err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}
	out := <-createDone
	if out.err != nil {
		t.Fatalf("CreatePlan: %v", out.err)
	}
	if out.result.Plan == nil || out.result.Plan.Content != "recreated" {
		t.Fatalf("expected the create to succeed once the delete released its lock, got %+v", out.result.Plan)
	}
}

// TestUpdatePlanHoldsLockAcrossConcurrentDelete is the mirror direction of
// TestDeletePlanHoldsLockAcrossConcurrentCreate, mandated by AC-002.12's
// Verification bullet: a plan *write* (UpdatePlan) held after its state read
// but before its commit, with a concurrent DeletePlan for the same task
// observed to queue rather than interleave. Confirms the per-task lock
// serializes this direction too, not just delete-holds/create-queues.
func TestUpdatePlanHoldsLockAcrossConcurrentDelete(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-update-del")

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-update-del", Title: "Plan", Content: "v0", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	updateDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-update-del", Content: "v1", AuthorKind: "agent", AuthorName: "A",
		})
		updateDone <- err
	}()
	<-writeStarted // UpdatePlan now holds the per-task lock, blocked inside its write tx.

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- svc.DeletePlan(ctx, "task-update-del")
	}()

	select {
	case err := <-deleteDone:
		t.Fatalf("concurrent DeletePlan finished (err=%v) before the gated UpdatePlan released its lock", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	if err := <-updateDone; err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}

	head, err := repo.GetTaskPlan(ctx, "task-update-del")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if head != nil {
		t.Fatalf("expected the delete (queued behind, then run after the update committed) to remove the row, got %+v", head)
	}
}

// TestUpdatePlanCancelledContextFailsAtWriteNotEarlierRead verifies AC-005.7:
// a context cancelled before the call still lets the pre-write tri-state
// read succeed (via context.WithoutCancel), and only the write transaction
// itself fails with context.Canceled. HEAD must be left unchanged.
func TestUpdatePlanCancelledContextFailsAtWriteNotEarlierRead(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-cancel")

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-cancel", Content: "v1"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.UpdatePlan(cancelledCtx, UpdatePlanRequest{TaskID: "task-cancel", Content: "v2"})
	if err == nil {
		t.Fatal("expected UpdatePlan to fail with an already-cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the error to wrap context.Canceled (from the write transaction, not an earlier read), got: %v", err)
	}

	head, getErr := repo.GetTaskPlan(ctx, "task-cancel")
	if getErr != nil {
		t.Fatalf("GetTaskPlan: %v", getErr)
	}
	if head == nil || head.Content != "v1" {
		t.Fatalf("expected HEAD unchanged by the failed write, got %+v", head)
	}
}

// TestUpdatePlanQueuedWriteCancelledWhileWaitingFailsAtItsOwnWriteAndDoesNotStrandTheLock
// covers the "while queued" half of AC-005.7 that
// TestUpdatePlanCancelledContextFailsAtWriteNotEarlierRead does not: writer A
// holds the per-task lock (gated inside its write tx), writer B is started
// concurrently and confirmed to be genuinely blocked waiting for that lock
// (not yet running), B's own context is cancelled while it is still queued,
// then A is released. B must still take its turn (its pre-write reads run on
// context.WithoutCancel so they are unaffected) and fail only once it reaches
// its own write transaction. A subsequent write to the same task must then
// succeed, proving B's cancellation did not strand the lock.
func TestUpdatePlanQueuedWriteCancelledWhileWaitingFailsAtItsOwnWriteAndDoesNotStrandTheLock(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-queued-cancel")

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-queued-cancel", Title: "Plan", Content: "v0", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	aDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-queued-cancel", Content: "from-a", AuthorKind: "agent", AuthorName: "A",
		})
		aDone <- err
	}()
	<-writeStarted // A holds the per-task lock, blocked inside its write tx.

	bCtx, cancelB := context.WithCancel(context.Background())
	bDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(bCtx, UpdatePlanRequest{
			TaskID: "task-queued-cancel", Content: "from-b", AuthorKind: "agent", AuthorName: "B",
		})
		bDone <- err
	}()
	waitForPlanLockWaiters(t, svc.locks, "task-queued-cancel", 2)

	cancelB() // cancel while B is still queued behind A, before A's lock is released

	select {
	case err := <-bDone:
		t.Fatalf("B returned (err=%v) immediately upon cancellation while still queued - it must wait its turn", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	if err := <-aDone; err != nil {
		t.Fatalf("A's UpdatePlan: %v", err)
	}

	bErr := <-bDone
	if bErr == nil {
		t.Fatal("expected B's UpdatePlan to fail once it reached its own write transaction with an already-cancelled context")
	}
	if !errors.Is(bErr, context.Canceled) {
		t.Fatalf("expected B's error to wrap context.Canceled (from its write transaction, not an earlier read), got: %v", bErr)
	}

	head, err := repo.GetTaskPlan(ctx, "task-queued-cancel")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if head == nil || head.Content != "from-a" {
		t.Fatalf("expected HEAD to still reflect A's committed write (B's cancelled write must not have applied), got %+v", head)
	}

	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-queued-cancel", Content: "from-c", AuthorKind: "agent", AuthorName: "C",
	}); err != nil {
		t.Fatalf("expected a subsequent write to the same task to succeed (B's cancellation must not strand the lock), got: %v", err)
	}
}

// TestCreatePlanPostWriteReReadFailureStillReportsWrittenPlan covers AC-005.8
// for the create/absent-head path: the pre-write read genuinely finds no
// row, the write commits a real INSERT, and only the post-write re-read
// fails. The in-memory plan (with its real, freshly-assigned ID) must still
// be reported rather than erroring out.
func TestCreatePlanPostWriteReReadFailureStillReportsWrittenPlan(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-flaky-create")

	svc := newFlakyPlanService(t, repo, eventBus, 2) // 1st GetTaskPlan (pre-write) succeeds, 2nd (post-write) fails
	result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-flaky-create", Title: "T", Content: "v1"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if result.Plan == nil || result.Plan.Content != "v1" || result.Plan.Title != "T" {
		t.Fatalf("expected the in-memory plan reflecting the committed write, got %+v", result.Plan)
	}
	if result.Plan.ID == "" {
		t.Error("expected a real ID from the fresh INSERT even though the post-write re-read failed (head was genuinely absent, not unknown)")
	}

	saved, err := repo.GetTaskPlan(ctx, "task-flaky-create")
	if err != nil {
		t.Fatalf("verify GetTaskPlan: %v", err)
	}
	if saved == nil || saved.ID != result.Plan.ID {
		t.Fatalf("expected the reported ID to match the actually-persisted row: saved=%+v reported=%+v", saved, result.Plan)
	}
}

// TestRevertPlanDoesNotAbortWhenOwnHeadReadFails covers AC-001.6/F36:
// RevertPlan never substitutes an empty request field for an existing HEAD
// value, so a failed HEAD read must not abort the revert - only clear the
// reported identity when the post-write re-read also fails.
func TestRevertPlanDoesNotAbortWhenOwnHeadReadFails(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-revert-flaky")

	svc := NewPlanService(repo, eventBus, log)
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-revert-flaky", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-revert-flaky", Content: "v2", AuthorKind: "agent", AuthorName: "Claude", ForceNewRevision: true,
	}); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	list, err := svc.ListRevisions(ctx, "task-revert-flaky")
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRevisions: err=%v len=%d", err, len(list))
	}
	var v1ID string
	for _, rev := range list {
		if rev.RevisionNumber == 1 {
			v1ID = rev.ID
		}
	}
	if v1ID == "" {
		t.Fatal("could not find revision 1")
	}

	flakySvc := newFlakyPlanService(t, repo, eventBus, 1) // every GetTaskPlan call fails
	rev, err := flakySvc.RevertPlan(ctx, RevertPlanRequest{
		TaskID: "task-revert-flaky", TargetRevisionID: v1ID, AuthorName: "Alice",
	})
	if err != nil {
		t.Fatalf("RevertPlan must not abort when its own HEAD read fails, got: %v", err)
	}
	if rev.Content != "v1" {
		t.Errorf("expected revert content v1, got %q", rev.Content)
	}

	head, err := repo.GetTaskPlan(ctx, "task-revert-flaky")
	if err != nil {
		t.Fatalf("verify GetTaskPlan: %v", err)
	}
	if head == nil || head.Content != "v1" {
		t.Fatalf("expected HEAD content v1 after revert, got %+v", head)
	}
}

// TestRevertPlanQueuedBehindHeldWriteCancelledWhileWaitingFailsAtItsOwnWriteAndDoesNotStrandTheLock
// is the RevertPlan-specific instance of AC-005.7's "while queued" shape:
// RevertPlan is queued behind another writer holding the per-task lock, its
// own context is cancelled while it waits, and it must still take its turn
// and fail only at its own write transaction (per F38's context.WithoutCancel
// placement on its target-revision fetch), leaving HEAD unchanged, with a
// subsequent write to the same task succeeding afterward.
func TestRevertPlanQueuedBehindHeldWriteCancelledWhileWaitingFailsAtItsOwnWriteAndDoesNotStrandTheLock(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-revert-queued-cancel")

	seedSvc := NewPlanService(repo, eventBus, log)
	if _, err := seedSvc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-revert-queued-cancel", Content: "v1", AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if _, err := seedSvc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-revert-queued-cancel", Content: "v2", AuthorKind: "agent", AuthorName: "Claude", ForceNewRevision: true,
	}); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	list, err := seedSvc.ListRevisions(ctx, "task-revert-queued-cancel")
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRevisions: err=%v len=%d", err, len(list))
	}
	var v1ID string
	for _, rev := range list {
		if rev.RevisionNumber == 1 {
			v1ID = rev.ID
		}
	}
	if v1ID == "" {
		t.Fatal("could not find revision 1")
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	aDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-revert-queued-cancel", Content: "from-a", AuthorKind: "agent", AuthorName: "A", ForceNewRevision: true,
		})
		aDone <- err
	}()
	<-writeStarted // A holds the per-task lock, blocked inside its write tx.

	revertCtx, cancelRevert := context.WithCancel(context.Background())
	revertDone := make(chan error, 1)
	go func() {
		_, err := svc.RevertPlan(revertCtx, RevertPlanRequest{
			TaskID: "task-revert-queued-cancel", TargetRevisionID: v1ID, AuthorName: "Reverter",
		})
		revertDone <- err
	}()
	waitForPlanLockWaiters(t, svc.locks, "task-revert-queued-cancel", 2)

	cancelRevert() // cancel while the revert is still queued behind A, before A's lock is released

	select {
	case err := <-revertDone:
		t.Fatalf("RevertPlan returned (err=%v) immediately upon cancellation while still queued - it must wait its turn", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	if err := <-aDone; err != nil {
		t.Fatalf("A's UpdatePlan: %v", err)
	}

	revertErr := <-revertDone
	if revertErr == nil {
		t.Fatal("expected the queued RevertPlan to fail once it reached its own write transaction with an already-cancelled context")
	}
	if !errors.Is(revertErr, context.Canceled) {
		t.Fatalf("expected the revert's error to wrap context.Canceled (from its write transaction, not the target-revision/HEAD reads), got: %v", revertErr)
	}

	head, err := repo.GetTaskPlan(ctx, "task-revert-queued-cancel")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if head == nil || head.Content != "from-a" {
		t.Fatalf("expected HEAD to still reflect A's committed write (the cancelled revert must not have applied), got %+v", head)
	}

	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-revert-queued-cancel", Content: "from-c", AuthorKind: "agent", AuthorName: "C", ForceNewRevision: true,
	}); err != nil {
		t.Fatalf("expected a subsequent write to the same task to succeed (the cancelled revert must not strand the lock), got: %v", err)
	}
}

// TestResolveCoalesceWindowPreservesNegativeAsNeverCoalesce covers AC-002.8:
// an explicitly configured negative window must not be clamped back up to
// the default, since a negative value is how a caller says "never coalesce".
func TestResolveCoalesceWindowPreservesNegativeAsNeverCoalesce(t *testing.T) {
	got := resolveCoalesceWindow(-1 * time.Second)
	if got != -1*time.Second {
		t.Fatalf("resolveCoalesceWindow(-1s) = %s, want -1s unchanged", got)
	}
}

// TestPlanService_NegativeCoalesceWindowNeverCoalesces is the integration
// counterpart of the unit test above: with a negative window configured,
// same-author writes within any interval must never merge.
func TestPlanService_NegativeCoalesceWindowNeverCoalesces(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-negwin")
	svc.coalesceWindow = -1 * time.Second

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-negwin", Content: "v1", AuthorKind: "agent", AuthorName: "Claude"}); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-negwin", Content: "v2", AuthorKind: "agent", AuthorName: "Claude"}); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	list, err := svc.ListRevisions(ctx, "task-negwin")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 separate revisions with a negative (never-coalesce) window, got %d", len(list))
	}
}

// TestPlanService_CoalescesRepeatedlyWithinWindow proves coalescing is not
// limited to a single merge: three same-author writes inside the window
// must all fold into the original revision, keeping its number and only the
// latest content.
func TestPlanService_CoalescesRepeatedlyWithinWindow(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-repeat-co")
	svc.coalesceWindow = 10 * time.Minute

	for i, content := range []string{"v1", "v2", "v3"} {
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
			TaskID: "task-repeat-co", Content: content, AuthorKind: "agent", AuthorName: "Claude",
		}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	list, err := svc.ListRevisions(ctx, "task-repeat-co")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected all 3 same-author within-window writes to coalesce into 1 revision, got %d", len(list))
	}
	if list[0].RevisionNumber != 1 {
		t.Errorf("expected the coalesced revision to keep number 1 across repeated merges, got %d", list[0].RevisionNumber)
	}

	full, err := svc.GetRevision(ctx, list[0].ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if full.Content != "v3" {
		t.Errorf("expected the coalesced revision to hold the latest (3rd) content, got %q", full.Content)
	}
}

// newSeparatePoolsPlanService builds a PlanService backed by genuinely
// separate writer and reader *sql.DB pools - the same internal/db.OpenSQLite
// / OpenSQLiteReader split production uses - rather than the single handle
// every other test in this package shares as both. AC-002.10 requires plan
// reads inside the serialized section to observe a commit made through a
// different database connection than the read uses; a shared handle can't
// exercise that, since a single connection trivially sees its own prior
// writes.
func newSeparatePoolsPlanService(t *testing.T) (*PlanService, *sqliterepo.Repository) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	writerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite (writer pool): %v", err)
	}
	readerConn, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteReader (reader pool): %v", err)
	}
	writerDB := sqlx.NewDb(writerConn, "sqlite3")
	readerDB := sqlx.NewDb(readerConn, "sqlite3")

	repo, cleanup, err := repository.Provide(writerDB, readerDB, nil)
	if err != nil {
		t.Fatalf("repository.Provide: %v", err)
	}
	t.Cleanup(func() {
		if err := writerDB.Close(); err != nil {
			t.Errorf("failed to close writer pool: %v", err)
		}
		if err := readerDB.Close(); err != nil {
			t.Errorf("failed to close reader pool: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})

	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewPlanService(repo, eventBus, log)
	return svc, repo
}

// TestPlanService_SeparateReaderWriterPoolsObserveCommittedWrites is the one
// test in this package wired with real separate connection pools (AC-002.10).
// It drives the service through the same read-after-write sequence
// upsertPlan performs inside the per-task lock - HEAD read, latest-revision
// read, write, re-read - and asserts each read observes what the previous
// write committed through the other pool, including the coalesce/append
// decision a second write makes from what it reads.
func TestPlanService_SeparateReaderWriterPoolsObserveCommittedWrites(t *testing.T) {
	svc, repo := newSeparatePoolsPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-pool-split")

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-pool-split", Title: "Plan", Content: "v1", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Read HEAD and the latest revision through the reader pool - a
	// connection distinct from the one the write above committed through.
	head, err := repo.GetTaskPlan(ctx, "task-pool-split")
	if err != nil {
		t.Fatalf("GetTaskPlan (reader pool): %v", err)
	}
	if head == nil || head.Content != "v1" {
		t.Fatalf("reader pool did not observe the write committed through the writer pool: got %+v", head)
	}
	rev, err := repo.GetLatestTaskPlanRevision(ctx, "task-pool-split")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision (reader pool): %v", err)
	}
	if rev == nil || rev.Content != "v1" || rev.RevisionNumber != 1 {
		t.Fatalf("reader pool did not observe the revision committed through the writer pool: got %+v", rev)
	}

	// A second write, forced to append, must compute its
	// PriorRevisionNumber and coalesce decision from what the reader pool
	// observes of the first write's commit - not from any writer-pool-local
	// state, since PlanService always reads via the reader pool.
	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-pool-split", Content: "v2", AuthorKind: "agent", AuthorName: "A", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}
	if result.Plan == nil || result.Plan.Content != "v2" {
		t.Fatalf("expected the second write to commit v2 based on a reader-pool read of the first write's commit, got %+v", result.Plan)
	}

	list, err := svc.ListRevisions(ctx, "task-pool-split")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 || list[0].RevisionNumber != 2 {
		t.Fatalf("expected the second write to append revision 2 (numbered from a reader-pool read of revision 1), got %+v", list)
	}
}

// TestPlanService_ConcurrentTruncatingWriteReflectsCommittedPredecessor is
// the spec's core TOCTOU scenario (AC-001.3, AC-002.1, AC-002.2, AC-002.3):
// writer A is gated inside its write transaction, already holding the
// per-task lock, after having read the seeded plan state. Writer B for the
// same task is started while A is still blocked and must queue rather than
// commit. Releasing A lets both commit in order, and each write's truncation
// decision (ReplacedRunes/NewRunes/PriorRevisionNumber) must describe the
// state that write actually committed against - B's decision must reflect
// A's committed content and revision number, not the stale seed state that
// existed before A ran, and HEAD must end up equal to the latest revision.
func TestPlanService_ConcurrentTruncatingWriteReflectsCommittedPredecessor(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-interleave-trunc")

	seedContent := strings.Repeat("s", 10000)
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-interleave-trunc", Title: "Plan", Content: seedContent, CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}
	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-interleave-trunc", RevisionNumber: 1, Title: "Plan", Content: seedContent,
		AuthorKind: "agent", AuthorName: "Seed",
	}); err != nil {
		t.Fatalf("seed InsertTaskPlanRevision: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	type writeOutcome struct {
		result PlanWriteResult
		err    error
	}

	aContent := strings.Repeat("a", 3000) // < half of seedContent's 10000 runes: A must detect truncation.
	aDone := make(chan writeOutcome, 1)
	go func() {
		result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-interleave-trunc", Content: aContent, AuthorKind: "agent", AuthorName: "A",
			EvaluateTruncation: true,
		})
		aDone <- writeOutcome{result, err}
	}()
	<-writeStarted // A holds the per-task lock, blocked inside its write tx, after having read the seed.

	bContent := strings.Repeat("b", 1000) // < half of aContent's 3000 runes, IF measured against A's commit.
	bDone := make(chan writeOutcome, 1)
	go func() {
		result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-interleave-trunc", Content: bContent, AuthorKind: "agent", AuthorName: "B",
			EvaluateTruncation: true,
		})
		bDone <- writeOutcome{result, err}
	}()

	select {
	case out := <-bDone:
		t.Fatalf("second write finished (err=%v) before the gated first write released the per-task lock - not queued", out.err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	aOut := <-aDone
	if aOut.err != nil {
		t.Fatalf("write A: %v", aOut.err)
	}
	bOut := <-bDone
	if bOut.err != nil {
		t.Fatalf("write B: %v", bOut.err)
	}

	if !aOut.result.TruncationDetected {
		t.Error("expected write A to detect truncation against the seeded content")
	}
	if aOut.result.ReplacedRunes != 10000 || aOut.result.NewRunes != 3000 {
		t.Errorf("write A: got ReplacedRunes=%d NewRunes=%d, want 10000/3000 (replaced the seed)",
			aOut.result.ReplacedRunes, aOut.result.NewRunes)
	}
	if aOut.result.PriorRevisionNumber != 1 {
		t.Errorf("write A: got PriorRevisionNumber=%d, want 1 (the seed revision)", aOut.result.PriorRevisionNumber)
	}

	if !bOut.result.TruncationDetected {
		t.Fatal("expected write B to detect truncation against A's committed content, not the stale seed it would have read had it not queued")
	}
	if bOut.result.ReplacedRunes != 3000 || bOut.result.NewRunes != 1000 {
		t.Errorf("write B: got ReplacedRunes=%d NewRunes=%d, want 3000/1000 (replaced A's committed content, not the 10000-rune seed)",
			bOut.result.ReplacedRunes, bOut.result.NewRunes)
	}
	if bOut.result.PriorRevisionNumber != 2 {
		t.Errorf("write B: got PriorRevisionNumber=%d, want 2 (A's committed revision, not the seed's revision 1)", bOut.result.PriorRevisionNumber)
	}

	head, err := svc.GetPlan(ctx, "task-interleave-trunc")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if head.Content != bContent {
		t.Errorf("expected HEAD to equal B's committed content (the last write), got a plan with %d bytes", len(head.Content))
	}

	list, err := svc.ListRevisions(ctx, "task-interleave-trunc")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 revisions (seed=1, A's truncating append=2, B's truncating append=3), got %d", len(list))
	}
	if list[0].RevisionNumber != 3 {
		t.Errorf("expected the newest revision to be numbered 3, got %d", list[0].RevisionNumber)
	}
	if list[1].RevisionNumber != 2 {
		t.Errorf("expected the middle revision to be numbered 2, got %d", list[1].RevisionNumber)
	}
}
