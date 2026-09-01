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

// TestPlanService_CreateAfterDeleteAppendsRatherThanCoalescesWithSurvivingRevisions
// covers AC-002.11: DeletePlan removes only the task_plans HEAD row, leaving
// prior revision rows in place. A same-author write inside the coalesce
// window immediately after a delete has no HEAD to coalesce into, so it must
// append a fresh revision rather than merging into a surviving revision from
// before the delete.
func TestPlanService_CreateAfterDeleteAppendsRatherThanCoalescesWithSurvivingRevisions(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-delete-recreate")
	svc.coalesceWindow = 10 * time.Minute

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-delete-recreate", Content: "v1", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("initial CreatePlan: %v", err)
	}
	if err := svc.DeletePlan(ctx, "task-delete-recreate"); err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}

	// Same author, immediately after (well inside the coalesce window): with
	// no HEAD, this must append rather than coalesce into the surviving
	// revision 1.
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-delete-recreate", Content: "v2", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("CreatePlan after delete: %v", err)
	}

	list, err := svc.ListRevisions(ctx, "task-delete-recreate")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected the post-delete write to append a second revision alongside the surviving first one, got %d revisions: %+v", len(list), list)
	}
	if list[0].RevisionNumber != 2 {
		t.Errorf("expected the post-delete write to be numbered 2 (appended, not coalesced into revision 1), got %d", list[0].RevisionNumber)
	}
	if list[1].RevisionNumber != 1 {
		t.Errorf("expected the pre-delete revision to survive as revision 1, got %d", list[1].RevisionNumber)
	}

	full, err := svc.GetRevision(ctx, list[1].ID)
	if err != nil {
		t.Fatalf("GetRevision(surviving revision 1): %v", err)
	}
	if full.Content != "v1" {
		t.Errorf("expected the surviving revision 1 to keep its original content, got %q", full.Content)
	}

	head, err := svc.GetPlan(ctx, "task-delete-recreate")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if head == nil || head.Content != "v2" {
		t.Fatalf("expected HEAD to be the post-delete write, got %+v", head)
	}
}

// failOnceWriteRepo makes WritePlanRevision fail exactly its first call,
// succeeding on every call after - simulating a transient commit failure
// (e.g. a busy database) that a caller retries.
type failOnceWriteRepo struct {
	*sqliterepo.Repository
	failed int32
}

func (r *failOnceWriteRepo) WritePlanRevision(ctx context.Context, head *models.TaskPlan, rev *models.TaskPlanRevision, coalesceLatestID *string, preserveTitle, preserveCreatedBy bool) error {
	if atomic.CompareAndSwapInt32(&r.failed, 0, 1) {
		return errors.New("simulated commit failure")
	}
	return r.Repository.WritePlanRevision(ctx, head, rev, coalesceLatestID, preserveTitle, preserveCreatedBy)
}

// TestPlanService_WriteFailureDoesNotStrandTheLock covers AC-005.3: a write
// that fails after it has begun (here, at the commit) must leave the task
// able to accept a subsequent write. Both writes go through the same
// PlanService instance - and therefore the same per-task lock table entry -
// so a hang on the retry would prove the failed write stranded the lock.
func TestPlanService_WriteFailureDoesNotStrandTheLock(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-fail-then-retry")

	failing := &failOnceWriteRepo{Repository: repo}
	svc := NewPlanService(failing, eventBus, log)

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-fail-then-retry", Content: "v1", AuthorKind: "agent", AuthorName: "A",
	}); err == nil {
		t.Fatal("expected the first (simulated commit-failure) write to fail")
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.CreatePlan(ctx, CreatePlanRequest{
			TaskID: "task-fail-then-retry", Content: "v2", AuthorKind: "agent", AuthorName: "A",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("retry after a write failure: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retry after a write failure hung - the failed write appears to have stranded the per-task lock")
	}

	plan, err := svc.GetPlan(ctx, "task-fail-then-retry")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan == nil || plan.Content != "v2" {
		t.Fatalf("expected the retry to have committed v2, got %+v", plan)
	}
}

// TestPlanService_IdenticalContentRepeatIsNotDeduplicated covers AC-005.1:
// plan writes are never deduplicated by content. A same-author repeat inside
// the coalesce window merges into the latest revision and adds none (the
// ordinary coalesce rule, incidentally true here since the content is also
// identical); a repeat by a different author (forcing append, same as any
// other write) must append a revision whose content equals its
// predecessor's rather than being silently dropped as a no-op.
func TestPlanService_IdenticalContentRepeatIsNotDeduplicated(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-identical-repeat")
	svc.coalesceWindow = 10 * time.Minute

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-identical-repeat", Content: "same", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-identical-repeat", Content: "same", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("write 2 (identical content, same author, in window): %v", err)
	}

	list, err := svc.ListRevisions(ctx, "task-identical-repeat")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected the identical-content repeat within the window to coalesce (add none), got %d revisions", len(list))
	}

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-identical-repeat", Content: "same", AuthorKind: "agent", AuthorName: "B",
	}); err != nil {
		t.Fatalf("write 3 (identical content, different author): %v", err)
	}

	list, err = svc.ListRevisions(ctx, "task-identical-repeat")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected the identical-content repeat by a different author to append a duplicate-content revision, got %d revisions", len(list))
	}
	if list[0].RevisionNumber != 2 {
		t.Errorf("expected the newly appended revision to be numbered 2, got %d", list[0].RevisionNumber)
	}

	full, err := svc.GetRevision(ctx, list[0].ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if full.Content != "same" {
		t.Errorf("expected the appended revision's content to equal its predecessor's (not deduplicated away), got %q", full.Content)
	}
}

// nilReadPlanRepo makes GetTaskPlan legitimately report "no row found" (nil,
// nil - not an error) from its callCount'th invocation onward (1-based). This
// is distinct from flakyPlanReadRepo, which forces an error: AC-005.8's
// Verification section requires both post-write-re-read outcomes be tested
// separately, since finalizePlanIdentity treats `err != nil` and `saved ==
// nil` as the same branch but they are reached by genuinely different
// conditions (a failed read vs. a read that legitimately finds nothing).
type nilReadPlanRepo struct {
	*sqliterepo.Repository
	callCount  int
	nilFromNth int
}

func (r *nilReadPlanRepo) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	r.callCount++
	if r.nilFromNth > 0 && r.callCount >= r.nilFromNth {
		return nil, nil
	}
	return r.Repository.GetTaskPlan(ctx, taskID)
}

// TestCreatePlanPostWriteReReadFindsNoRowStillReportsWrittenPlan covers the
// second half of AC-005.8 for the create/absent-head path: the pre-write read
// genuinely finds no row, the write commits a real INSERT, and the post-write
// re-read also genuinely finds no row (not an error). The in-memory plan
// (with its real, freshly-assigned ID) must still be reported as successful,
// exactly as the sibling error-injection test
// (TestCreatePlanPostWriteReReadFailureStillReportsWrittenPlan) already
// covers for the error case.
func TestCreatePlanPostWriteReReadFindsNoRowStillReportsWrittenPlan(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-nilread-create")

	// 1st GetTaskPlan (pre-write) hits the real repo and finds no row; the
	// 2nd (post-write re-read) is forced to legitimately return (nil, nil).
	nilRead := &nilReadPlanRepo{Repository: repo, nilFromNth: 2}
	svc := NewPlanService(nilRead, eventBus, log)

	result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-nilread-create", Title: "T", Content: "v1"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if result.Plan == nil || result.Plan.Content != "v1" || result.Plan.Title != "T" {
		t.Fatalf("expected the in-memory plan reflecting the committed write, got %+v", result.Plan)
	}
	if result.Plan.ID == "" {
		t.Error("expected a real ID from the fresh INSERT even though the post-write re-read legitimately found no row (head was genuinely absent, not unknown)")
	}

	saved, err := repo.GetTaskPlan(ctx, "task-nilread-create")
	if err != nil {
		t.Fatalf("verify GetTaskPlan: %v", err)
	}
	if saved == nil || saved.ID != result.Plan.ID {
		t.Fatalf("expected the reported ID to match the actually-persisted row: saved=%+v reported=%+v", saved, result.Plan)
	}
}

// panicOnDeleteRepo makes DeleteTaskPlan panic instead of returning, simulating
// an unrecoverable failure (e.g. a driver bug or invariant violation) partway
// through the delete that DeletePlan's own error-return branches cannot
// anticipate.
type panicOnDeleteRepo struct {
	*sqliterepo.Repository
}

func (r *panicOnDeleteRepo) DeleteTaskPlan(ctx context.Context, taskID string) error {
	panic("simulated panic inside DeleteTaskPlan")
}

// TestDeletePlanPanicDuringDeleteDoesNotStrandTheLock proves DeletePlan's
// per-task lock is released even when it exits via panic rather than one of
// its own return statements. DeletePlan's explicit release() calls sit only
// on its named return paths, so a panic between acquiring the lock and
// reaching any of them would previously skip every one of them and strand the
// lock forever - every future write to that task queues indefinitely, with no
// self-heal, since a panic inside an HTTP handler is typically recovered
// per-request rather than crashing the process. Both the panicking call and
// the follow-up write share the same PlanService instance, and therefore the
// same per-task lock table entry, so a hang on the follow-up proves the
// stranding.
func TestDeletePlanPanicDuringDeleteDoesNotStrandTheLock(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-delete-panic")

	panicking := &panicOnDeleteRepo{Repository: repo}
	svc := NewPlanService(panicking, eventBus, log)

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-delete-panic", Content: "v1", AuthorKind: "agent", AuthorName: "A",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	recovered := make(chan any, 1)
	func() {
		defer func() { recovered <- recover() }()
		_ = svc.DeletePlan(ctx, "task-delete-panic")
	}()
	if r := <-recovered; r == nil {
		t.Fatal("expected DeletePlan to panic via the injected DeleteTaskPlan failure")
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-delete-panic", Content: "after-panic", AuthorKind: "agent", AuthorName: "A",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write after a panicking delete: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write after a panicking delete hung - the panic appears to have stranded the per-task lock")
	}

	plan, err := svc.GetPlan(ctx, "task-delete-panic")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan == nil || plan.Content != "after-panic" {
		t.Fatalf("expected the follow-up write to have committed, got %+v", plan)
	}
}
