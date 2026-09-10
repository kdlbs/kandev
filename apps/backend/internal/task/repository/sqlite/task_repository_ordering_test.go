package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie pins
// AC-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001.2: position and created_at alone
// do not total-order the attachment set, so two rows sharing both must still
// resolve to one defined primary. GetPrimaryTaskRepository must agree with
// ListTaskRepositories, since it is defined as that read's first row.
func TestListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-ordering-tiebreak")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-ordering-tiebreak", WorkspaceID: "ws-ordering-tiebreak", Name: "Workflow",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-ordering-tiebreak", WorkspaceID: "ws-ordering-tiebreak",
		WorkflowID: "wf-ordering-tiebreak", Title: "Task",
	}); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []*models.Repository{
		{ID: "repo-ordering-a", WorkspaceID: "ws-ordering-tiebreak", Name: "a"},
		{ID: "repo-ordering-b", WorkspaceID: "ws-ordering-tiebreak", Name: "b"},
	} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatal(err)
		}
	}

	// Both rows share position and created_at, so only the id ASC tiebreak
	// can order them. task-repository-b is inserted second but must still
	// sort first: it is lexicographically smaller.
	tiedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct{ id, repositoryID string }{
		{"task-repository-c", "repo-ordering-a"},
		{"task-repository-b", "repo-ordering-b"},
	} {
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO task_repositories (id, task_id, repository_id, position, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?)
		`), row.id, "task-ordering-tiebreak", row.repositoryID, tiedAt, tiedAt); err != nil {
			t.Fatalf("insert tied task repository %s: %v", row.id, err)
		}
	}

	got, err := repo.ListTaskRepositories(ctx, "task-ordering-tiebreak")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(got) != 2 || got[0].ID != "task-repository-b" || got[1].ID != "task-repository-c" {
		t.Fatalf("expected id-ascending tiebreak [task-repository-b task-repository-c], got %+v", got)
	}

	primary, err := repo.GetPrimaryTaskRepository(ctx, "task-ordering-tiebreak")
	if err != nil {
		t.Fatalf("GetPrimaryTaskRepository: %v", err)
	}
	if primary == nil || primary.ID != "task-repository-b" {
		t.Fatalf("GetPrimaryTaskRepository = %+v, want task-repository-b (the same row ListTaskRepositories[0] returns)", primary)
	}
}
