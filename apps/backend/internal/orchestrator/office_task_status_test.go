package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// recordingOfficeStatusUpdater is a test double for OfficeTaskStatusUpdater
// that performs the same kind of write UpdateTaskStatus does (persist a
// state via the shared repo) and returns a configurable error, so tests
// can observe both what the orchestrator asked for and what state ends
// up persisted — without standing up the full dashboard service.
type recordingOfficeStatusUpdater struct {
	repo     *sqliterepo.Repository
	newState v1.TaskState // state to persist; defaults to COMPLETED
	err      error        // returned after the write, mirrors UpdateTaskStatus's contract
	calls    []dashboard.TaskStatusUpdateRequest
}

func (u *recordingOfficeStatusUpdater) UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error {
	u.calls = append(u.calls, req)
	state := v1.TaskStateCompleted
	if u.newState != "" {
		state = u.newState
	}
	task, err := u.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return err
	}
	task.State = state
	task.UpdatedAt = time.Now().UTC()
	if err := u.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	return u.err
}

// requireCreateOfficeTask creates a task with a non-empty ProjectID so the
// repo's IsFromOffice projection reports true (see isFromOfficeProjection).
func requireCreateOfficeTask(t *testing.T, repo *sqliterepo.Repository, task *models.Task) {
	t.Helper()
	if task.ProjectID == "" {
		task.ProjectID = "proj-office"
	}
	requireCreateTask(t, repo, task)
}

// TestMarkOfficeTaskCompleted_NoApprovers_WritesDoneViaSeam is test-matrix
// case (4): an orchestrator terminal-step move for an Office task with no
// approvers pending routes through the seam and ends up COMPLETED.
func TestMarkOfficeTaskCompleted_NoApprovers_WritesDoneViaSeam(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-done", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-done", "child_done")

	if len(updater.calls) != 1 || updater.calls[0].NewStatus != "done" {
		t.Fatalf("seam calls = %+v, want one call with NewStatus=done", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-done")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED", task.State)
	}
}

// TestMarkOfficeTaskCompleted_ApproversPending_RedirectsToReview is
// test-matrix case (3): pending approvers redirect the persisted state to
// in_review (REVIEW), and the ApprovalsPendingError the seam returns is
// not a failure — it must not trigger the raw completion write.
func TestMarkOfficeTaskCompleted_ApproversPending_RedirectsToReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-pending", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{
		repo:     repo,
		newState: v1.TaskStateReview,
		err:      &dashboard.ApprovalsPendingError{Pending: []string{"agent-A"}},
	}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-pending", "child_done")

	if len(updater.calls) != 1 || updater.calls[0].NewStatus != "done" {
		t.Fatalf("seam calls = %+v, want one call with NewStatus=done", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-pending")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Fatalf("state = %q, want REVIEW (redirected, not overwritten by the raw completion path)", task.State)
	}
}

// TestMarkTaskCompleted_NonOfficeTask_UsesRawPathUnchanged is test-matrix
// case (c): a non-Office task's terminal-step completion is untouched by
// this change — it never reaches the seam and keeps the raw write.
func TestMarkTaskCompleted_NonOfficeTask_UsesRawPathUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "kanban-task", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Kanban task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "kanban-task", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none: non-Office tasks must not reach the Office seam", updater.calls)
	}
	task, err := repo.GetTask(ctx, "kanban-task")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED via the unchanged raw path", task.State)
	}
}

// TestMarkOfficeTaskCompleted_SeamNil_SkipsWrite is test-matrix case (d):
// with no seam wired, an Office task's terminal completion is skipped
// rather than falling back to the raw write, which would bypass the
// approval gate.
func TestMarkOfficeTaskCompleted_SeamNil_SkipsWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-no-seam", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	// No SetOfficeTaskStatusUpdater call: the seam is nil.

	svc.markTaskCompletedForTerminalStep(ctx, "office-no-seam", "child_done")

	task, err := repo.GetTask(ctx, "office-no-seam")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want unchanged IN_PROGRESS: a nil seam must skip the write, not fall through", task.State)
	}
}
