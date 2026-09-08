package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const reviewTestWorkspaceID = "ws-review"

func seedReviewTask(t *testing.T, ctx context.Context, repo *Repository, taskID string) {
	t.Helper()
	const wfID = "wf-review"
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: reviewTestWorkspaceID, Name: "Review WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: reviewTestWorkspaceID, Name: "WF"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: reviewTestWorkspaceID,
		WorkflowID:  wfID,
		Title:       "review task",
		State:       "BACKLOG",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}

func newReviewRun(t *testing.T, ctx context.Context, repo *Repository, taskID string) *models.TaskReviewRun {
	t.Helper()
	run := &models.TaskReviewRun{
		TaskID:    taskID,
		SessionID: "sess-1",
		Trigger:   models.ReviewTriggerManual,
		AgentID:   "claude-acp",
		Model:     "claude-haiku-4-5",
	}
	if err := repo.CreateTaskReviewRun(ctx, run); err != nil {
		t.Fatalf("CreateTaskReviewRun: %v", err)
	}
	return run
}

func finding(runID, taskID, path, title string, line int) *models.TaskReviewFinding {
	return &models.TaskReviewFinding{
		RunID:        runID,
		TaskID:       taskID,
		FilePath:     path,
		StartLine:    line,
		EndLine:      line,
		Severity:     models.ReviewSeverityMajor,
		Category:     "correctness",
		Title:        title,
		Body:         "body",
		FileDiffHash: "abc123",
	}
}

func TestTaskReviewRun_CreateGetUpdate(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-run")

	run := newReviewRun(t, ctx, repo, "task-run")
	if run.ID == "" || run.CreatedAt.IsZero() {
		t.Fatalf("expected id + created_at populated: %+v", run)
	}
	if run.Status != models.ReviewRunPending {
		t.Fatalf("expected default pending status, got %q", run.Status)
	}

	run.Status = models.ReviewRunCompleted
	run.FindingCount = 3
	run.FileCount = 2
	run.RepositoryCount = 1
	run.Summary = "reviewed 2 files"
	if err := repo.UpdateTaskReviewRun(ctx, run); err != nil {
		t.Fatalf("UpdateTaskReviewRun: %v", err)
	}

	got, err := repo.GetTaskReviewRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetTaskReviewRun: %v", err)
	}
	if got.Status != models.ReviewRunCompleted || got.FindingCount != 3 || got.Summary != "reviewed 2 files" {
		t.Fatalf("run round-trip mismatch: %+v", got)
	}
	if got.Trigger != models.ReviewTriggerManual {
		t.Fatalf("expected manual trigger, got %q", got.Trigger)
	}
}

// TestTaskReviewRun_CreateDuplicateEntryIDReturnsConflictSentinel locks in
// the documented behavior of ErrTaskReviewRunEntryConflict: a second insert
// with the same non-empty EntryID is rejected via the sentinel, matching the
// unique index idx_task_review_runs_entry_id, and the first row's EntryID
// remains resolvable by FindTaskReviewRunByEntryID.
func TestTaskReviewRun_CreateDuplicateEntryIDReturnsConflictSentinel(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-entry-conflict")

	first := &models.TaskReviewRun{
		TaskID:    "task-entry-conflict",
		SessionID: "sess-1",
		Trigger:   models.ReviewTriggerWorkflowStep,
		AgentID:   "claude-acp",
		Model:     "claude-haiku-4-5",
		EntryID:   "entry-conflict-1",
	}
	if err := repo.CreateTaskReviewRun(ctx, first); err != nil {
		t.Fatalf("CreateTaskReviewRun (first): %v", err)
	}

	second := &models.TaskReviewRun{
		TaskID:    "task-entry-conflict",
		SessionID: "sess-1",
		Trigger:   models.ReviewTriggerWorkflowStep,
		AgentID:   "claude-acp",
		Model:     "claude-haiku-4-5",
		EntryID:   "entry-conflict-1",
	}
	err := repo.CreateTaskReviewRun(ctx, second)
	if !errors.Is(err, ErrTaskReviewRunEntryConflict) {
		t.Fatalf("expected ErrTaskReviewRunEntryConflict, got %v", err)
	}

	got, err := repo.FindTaskReviewRunByEntryID(ctx, "entry-conflict-1")
	if err != nil {
		t.Fatalf("FindTaskReviewRunByEntryID: %v", err)
	}
	if got == nil || got.ID != first.ID {
		t.Fatalf("expected the winner's row (%s) to remain resolvable by entry id, got %+v", first.ID, got)
	}
}

func TestTaskReviewRun_GetMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	_, err := repo.GetTaskReviewRun(ctx, "nope")
	if !errors.Is(err, models.ErrTaskReviewRunNotFound) {
		t.Fatalf("expected ErrTaskReviewRunNotFound, got %v", err)
	}
	if err := repo.UpdateTaskReviewRun(ctx, &models.TaskReviewRun{ID: "nope"}); !errors.Is(err, models.ErrTaskReviewRunNotFound) {
		t.Fatalf("expected ErrTaskReviewRunNotFound on update, got %v", err)
	}
}

func TestTaskReviewFindings_CreateListOrdering(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-f")
	run := newReviewRun(t, ctx, repo, "task-f")

	batch := []*models.TaskReviewFinding{
		finding(run.ID, "task-f", "b.go", "second", 20),
		finding(run.ID, "task-f", "a.go", "first", 5),
	}
	batch[0].RepositoryName = "backend"
	batch[1].RepositoryName = "backend"
	if err := repo.CreateTaskReviewFindings(ctx, batch); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}

	got, err := repo.ListTaskReviewFindings(ctx, "task-f")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got))
	}
	if got[0].FilePath != "a.go" || got[1].FilePath != "b.go" {
		t.Fatalf("expected repository/file/line ordering, got %q then %q", got[0].FilePath, got[1].FilePath)
	}
	if got[0].Status != models.ReviewFindingOpen || got[0].Side != models.ReviewSideAdditions {
		t.Fatalf("expected open/additions defaults: %+v", got[0])
	}
}

func TestTaskReviewFindings_CreateEmptyIsNoop(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if err := repo.CreateTaskReviewFindings(ctx, nil); err != nil {
		t.Fatalf("expected nil error for empty batch, got %v", err)
	}
}

func TestTaskReviewFinding_UpdateStatus(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-s")
	run := newReviewRun(t, ctx, repo, "task-s")

	f := finding(run.ID, "task-s", "a.go", "issue", 4)
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{f}); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}

	now := f.CreatedAt
	if err := repo.UpdateTaskReviewFindingStatus(ctx, f.ID, models.ReviewFindingResolved, &now); err != nil {
		t.Fatalf("UpdateTaskReviewFindingStatus: %v", err)
	}
	got, err := repo.GetTaskReviewFinding(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetTaskReviewFinding: %v", err)
	}
	if got.Status != models.ReviewFindingResolved || got.ResolvedAt == nil {
		t.Fatalf("expected resolved with resolved_at, got %+v", got)
	}

	if err := repo.UpdateTaskReviewFindingStatus(ctx, f.ID, models.ReviewFindingOpen, nil); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err = repo.GetTaskReviewFinding(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetTaskReviewFinding after reopen: %v", err)
	}
	if got.Status != models.ReviewFindingOpen || got.ResolvedAt != nil {
		t.Fatalf("expected reopen to clear resolved_at, got %+v", got)
	}

	err = repo.UpdateTaskReviewFindingStatus(ctx, "missing", models.ReviewFindingOpen, nil)
	if !errors.Is(err, models.ErrTaskReviewFindingNotFound) {
		t.Fatalf("expected ErrTaskReviewFindingNotFound, got %v", err)
	}
}

func TestTaskReviewFindings_SupersedeOnlyOpenFromOtherRuns(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-sup")

	oldRun := newReviewRun(t, ctx, repo, "task-sup")
	sameAnchor := finding(oldRun.ID, "task-sup", "a.go", "duplicate issue", 10)
	resolvedSame := finding(oldRun.ID, "task-sup", "a.go", "duplicate issue", 10)
	resolvedSame.Status = models.ReviewFindingResolved
	other := finding(oldRun.ID, "task-sup", "a.go", "different issue", 10)
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{sameAnchor, resolvedSame, other}); err != nil {
		t.Fatalf("seed findings: %v", err)
	}

	newRun := newReviewRun(t, ctx, repo, "task-sup")
	fresh := finding(newRun.ID, "task-sup", "a.go", "duplicate issue", 10)
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{fresh}); err != nil {
		t.Fatalf("create fresh finding: %v", err)
	}

	deleted, err := repo.DeleteSupersededTaskReviewFindings(ctx, "task-sup", newRun.ID, SupersedeKeysFor([]*models.TaskReviewFinding{fresh}))
	if err != nil {
		t.Fatalf("DeleteSupersededTaskReviewFindings: %v", err)
	}
	// The exact ids matter, not just the count: a connected client holds the old
	// findings in memory and needs to know which to drop.
	if len(deleted) != 1 {
		t.Fatalf("expected exactly the open duplicate deleted, got %v", deleted)
	}
	if deleted[0] != sameAnchor.ID {
		t.Fatalf("expected the open duplicate's id returned, got %q want %q", deleted[0], sameAnchor.ID)
	}

	remaining, err := repo.ListTaskReviewFindings(ctx, "task-sup")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected resolved duplicate + different issue + fresh to remain, got %d", len(remaining))
	}
	for _, f := range remaining {
		if f.ID == sameAnchor.ID {
			t.Fatalf("open duplicate from the earlier run should have been superseded")
		}
	}
}

func TestTaskReviewFindings_SupersedeNoKeysIsNoop(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	deleted, err := repo.DeleteSupersededTaskReviewFindings(ctx, "task", "run", nil)
	if err != nil || len(deleted) != 0 {
		t.Fatalf("expected no-op, got deleted=%v err=%v", deleted, err)
	}
}

func TestSupersedeKeysFor_Deduplicates(t *testing.T) {
	a := finding("r", "t", "a.go", "same", 3)
	b := finding("r", "t", "a.go", "same", 3)
	c := finding("r", "t", "a.go", "other", 3)

	keys := SupersedeKeysFor([]*models.TaskReviewFinding{a, b, c})
	if len(keys) != 2 {
		t.Fatalf("expected 2 distinct keys, got %d: %+v", len(keys), keys)
	}
}

func TestCancelInFlightTaskReviewRuns(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-c")

	pending := newReviewRun(t, ctx, repo, "task-c")
	running := newReviewRun(t, ctx, repo, "task-c")
	running.Status = models.ReviewRunRunning
	if err := repo.UpdateTaskReviewRun(ctx, running); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	done := newReviewRun(t, ctx, repo, "task-c")
	done.Status = models.ReviewRunCompleted
	if err := repo.UpdateTaskReviewRun(ctx, done); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	active, err := repo.ListActiveTaskReviewRuns(ctx, "task-c")
	if err != nil {
		t.Fatalf("ListActiveTaskReviewRuns: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active runs, got %d", len(active))
	}

	cancelled, err := repo.CancelInFlightTaskReviewRuns(ctx)
	if err != nil {
		t.Fatalf("CancelInFlightTaskReviewRuns: %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("expected 2 cancelled, got %d", cancelled)
	}

	for _, id := range []string{pending.ID, running.ID} {
		got, getErr := repo.GetTaskReviewRun(ctx, id)
		if getErr != nil {
			t.Fatalf("GetTaskReviewRun: %v", getErr)
		}
		if got.Status != models.ReviewRunCancelled || got.ErrorMessage != restartCancelReason {
			t.Fatalf("expected cancelled with restart reason, got %+v", got)
		}
		if got.CompletedAt == nil {
			t.Fatalf("expected completed_at set on cancel")
		}
	}
	stillDone, err := repo.GetTaskReviewRun(ctx, done.ID)
	if err != nil {
		t.Fatalf("GetTaskReviewRun: %v", err)
	}
	if stillDone.Status != models.ReviewRunCompleted {
		t.Fatalf("terminal run must not be re-cancelled, got %q", stillDone.Status)
	}
}

func TestListTaskReviewRuns_NewestFirstAndCapped(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-l")

	for range 3 {
		newReviewRun(t, ctx, repo, "task-l")
	}
	runs, err := repo.ListTaskReviewRuns(ctx, "task-l", 2)
	if err != nil {
		t.Fatalf("ListTaskReviewRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected limit honored, got %d", len(runs))
	}
	if runs[0].CreatedAt.Before(runs[1].CreatedAt) {
		t.Fatalf("expected newest-first ordering")
	}
}

func TestDeleteTaskReview_ByTaskAndByWorkspace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-d")
	run := newReviewRun(t, ctx, repo, "task-d")
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{
		finding(run.ID, "task-d", "a.go", "issue", 2),
	}); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}

	if err := repo.DeleteTaskReviewByTask(ctx, "task-d"); err != nil {
		t.Fatalf("DeleteTaskReviewByTask: %v", err)
	}
	findings, err := repo.ListTaskReviewFindings(ctx, "task-d")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected findings cleared, got %d", len(findings))
	}
	runs, err := repo.ListTaskReviewRuns(ctx, "task-d", 10)
	if err != nil {
		t.Fatalf("ListTaskReviewRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected runs cleared, got %d", len(runs))
	}

	// Re-seed and clear via the workspace cascade the E2E reset uses.
	run = newReviewRun(t, ctx, repo, "task-d")
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{
		finding(run.ID, "task-d", "a.go", "issue", 2),
	}); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}
	if err := repo.DeleteTaskReviewByWorkspace(ctx, reviewTestWorkspaceID); err != nil {
		t.Fatalf("DeleteTaskReviewByWorkspace: %v", err)
	}
	findings, err = repo.ListTaskReviewFindings(ctx, "task-d")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected workspace cascade to clear findings, got %d", len(findings))
	}
}

func TestTaskReview_CascadesOnTaskDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-cascade")
	run := newReviewRun(t, ctx, repo, "task-cascade")
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{
		finding(run.ID, "task-cascade", "a.go", "issue", 2),
	}); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}

	if err := repo.DeleteTask(ctx, "task-cascade"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	findings, err := repo.ListTaskReviewFindings(ctx, "task-cascade")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected findings removed with the task, got %d", len(findings))
	}
	runs, err := repo.ListTaskReviewRuns(ctx, "task-cascade", 10)
	if err != nil {
		t.Fatalf("ListTaskReviewRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected runs removed with the task, got %d", len(runs))
	}
}
