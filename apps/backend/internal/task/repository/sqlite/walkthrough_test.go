package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedWalkthroughTask(t *testing.T, ctx context.Context, repo *Repository, taskID string) {
	t.Helper()
	const wsID = "ws-wt"
	const wfID = "wf-wt"
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "WT WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "WF"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: wsID,
		WorkflowID:  wfID,
		Title:       "t",
		State:       "BACKLOG",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}

func TestWalkthroughRepo_CreateGet(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWalkthroughTask(t, ctx, repo, "task-1")

	wt := &models.TaskWalkthrough{
		TaskID: "task-1",
		Title:  "Tour",
		Steps: []models.WalkthroughStep{
			{Title: "Start", Repo: "main", File: "a.go", Line: 3, Text: "first"},
			{File: "b.go", Line: 9, LineEnd: 12, Text: "second"},
		},
	}
	if err := repo.CreateTaskWalkthrough(ctx, wt); err != nil {
		t.Fatalf("CreateTaskWalkthrough: %v", err)
	}
	if wt.ID == "" || wt.CreatedAt.IsZero() {
		t.Fatalf("expected id+timestamps populated: %+v", wt)
	}

	got, err := repo.GetTaskWalkthrough(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskWalkthrough: %v", err)
	}
	if got == nil || got.Title != "Tour" || len(got.Steps) != 2 {
		t.Fatalf("unexpected walkthrough: %+v", got)
	}
	if got.Steps[1].File != "b.go" || got.Steps[1].LineEnd != 12 {
		t.Fatalf("steps round-trip mismatch: %+v", got.Steps)
	}
}

func TestWalkthroughRepo_Upsert(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWalkthroughTask(t, ctx, repo, "task-1")

	first := &models.TaskWalkthrough{TaskID: "task-1", Steps: []models.WalkthroughStep{{File: "a.go", Line: 1, Text: "x"}}}
	if err := repo.CreateTaskWalkthrough(ctx, first); err != nil {
		t.Fatalf("create: %v", err)
	}
	second := &models.TaskWalkthrough{ID: first.ID, TaskID: "task-1", Steps: []models.WalkthroughStep{{File: "b.go", Line: 2, Text: "y"}, {File: "c.go", Line: 3, Text: "z"}}}
	if err := repo.CreateTaskWalkthrough(ctx, second); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("expected upsert to reuse persisted id+created_at: first=%+v second=%+v", first, second)
	}

	got, _ := repo.GetTaskWalkthrough(ctx, "task-1")
	if got == nil || len(got.Steps) != 2 || got.Steps[0].File != "b.go" {
		t.Fatalf("upsert did not replace steps: %+v", got)
	}
}

func TestWalkthroughRepo_GetMissing(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWalkthroughTask(t, ctx, repo, "task-1")

	got, err := repo.GetTaskWalkthrough(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskWalkthrough: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestWalkthroughRepo_Delete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWalkthroughTask(t, ctx, repo, "task-1")

	if err := repo.CreateTaskWalkthrough(ctx, &models.TaskWalkthrough{TaskID: "task-1", Steps: []models.WalkthroughStep{{File: "a.go", Line: 1, Text: "x"}}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.DeleteTaskWalkthrough(ctx, "task-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := repo.GetTaskWalkthrough(ctx, "task-1")
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
	if err := repo.DeleteTaskWalkthrough(ctx, "task-1"); !errors.Is(err, models.ErrTaskWalkthroughNotFound) {
		t.Fatalf("expected ErrTaskWalkthroughNotFound deleting missing walkthrough, got %v", err)
	}
}

func TestWalkthroughRepo_CascadeOnTaskDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWalkthroughTask(t, ctx, repo, "task-1")

	if err := repo.CreateTaskWalkthrough(ctx, &models.TaskWalkthrough{TaskID: "task-1", Steps: []models.WalkthroughStep{{File: "a.go", Line: 1, Text: "x"}}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	got, err := repo.GetTaskWalkthrough(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskWalkthrough after task delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected walkthrough cascade-deleted, got %+v", got)
	}
}
