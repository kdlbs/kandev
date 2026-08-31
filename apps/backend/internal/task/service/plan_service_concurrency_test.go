package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

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

	close(releaseWrite)
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

	close(releaseWrite)
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

	close(releaseDelete)
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

// TestUpdatePlanPostWriteReReadFailureClearsFabricatedIdentityWhenHeadWasUnknown
// covers AC-005.8 for the unknown-head path: both the pre-write HEAD read and
// the post-write re-read fail, so upsertPlan cannot tell whether its
// freshly-generated ID was actually used (a real row already existed and the
// write took the ON CONFLICT branch). The reported plan's ID/CreatedAt must
// be cleared, but the write itself must still have committed correctly,
// including preserving the existing title via preserveTitle (AC-001.9).
func TestUpdatePlanPostWriteReReadFailureClearsFabricatedIdentityWhenHeadWasUnknown(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-flaky-update")

	seeded := &models.TaskPlan{TaskID: "task-flaky-update", Title: "Original", Content: "v1", CreatedBy: "agent"}
	if err := repo.CreateTaskPlan(ctx, seeded); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	svc := newFlakyPlanService(t, repo, eventBus, 1) // every GetTaskPlan call fails
	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-flaky-update", Content: "v2", CreatedBy: "agent"})
	if err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}
	if result.Plan.ID != "" {
		t.Errorf("expected a cleared (unknown) ID after both reads failed, got %q", result.Plan.ID)
	}
	if !result.Plan.CreatedAt.IsZero() {
		t.Errorf("expected a cleared (unknown) CreatedAt, got %v", result.Plan.CreatedAt)
	}
	if result.Plan.Content != "v2" {
		t.Errorf("expected the in-memory plan to still report the content actually written, got %q", result.Plan.Content)
	}

	saved, err := repo.GetTaskPlan(ctx, "task-flaky-update")
	if err != nil {
		t.Fatalf("verify GetTaskPlan: %v", err)
	}
	if saved == nil || saved.Content != "v2" {
		t.Fatalf("expected the write to have committed despite unreadable state, got %+v", saved)
	}
	if saved.Title != "Original" {
		t.Errorf("expected the existing title preserved when the request omitted one and HEAD state was unknown, got %q", saved.Title)
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
