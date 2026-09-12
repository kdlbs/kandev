package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie is the
// Postgres counterpart to TestListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie.
// ListTaskRepositories' ORDER BY gained an `id ASC` tiebreak (AC-...-001.2); this
// pins that the tiebreak really does run on Postgres, per the AGENTS.md
// requirement that every changed dialect-sensitive method in this package get an
// environment-gated PostgreSQL behavior test. Skips unless KANDEV_TEST_POSTGRES_DSN
// is set.
func TestPostgresListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-ordering-tiebreak", Name: "ws-pg-ordering-tiebreak"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-pg-ordering-tiebreak", WorkspaceID: "ws-pg-ordering-tiebreak", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-pg-ordering-tiebreak", WorkspaceID: "ws-pg-ordering-tiebreak",
		WorkflowID: "wf-pg-ordering-tiebreak", Title: "Task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, repository := range []*models.Repository{
		{ID: "repo-pg-ordering-a", WorkspaceID: "ws-pg-ordering-tiebreak", Name: "a"},
		{ID: "repo-pg-ordering-b", WorkspaceID: "ws-pg-ordering-tiebreak", Name: "b"},
	} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("CreateRepository %s: %v", repository.ID, err)
		}
	}

	// Both rows share position and created_at, so only the id ASC tiebreak can
	// order them. task-repository-b is inserted second but must still sort
	// first: it is lexicographically smaller.
	tiedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct{ id, repositoryID string }{
		{"task-repository-pg-c", "repo-pg-ordering-a"},
		{"task-repository-pg-b", "repo-pg-ordering-b"},
	} {
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO task_repositories (id, task_id, repository_id, position, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?)
		`), row.id, "task-pg-ordering-tiebreak", row.repositoryID, tiedAt, tiedAt); err != nil {
			t.Fatalf("insert tied task repository %s: %v", row.id, err)
		}
	}

	got, err := repo.ListTaskRepositories(ctx, "task-pg-ordering-tiebreak")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(got) != 2 || got[0].ID != "task-repository-pg-b" || got[1].ID != "task-repository-pg-c" {
		t.Fatalf("expected id-ascending tiebreak [task-repository-pg-b task-repository-pg-c], got %+v", got)
	}

	primary, err := repo.GetPrimaryTaskRepository(ctx, "task-pg-ordering-tiebreak")
	if err != nil {
		t.Fatalf("GetPrimaryTaskRepository: %v", err)
	}
	if primary == nil || primary.ID != "task-repository-pg-b" {
		t.Fatalf("GetPrimaryTaskRepository = %+v, want task-repository-pg-b (the same row ListTaskRepositories[0] returns)", primary)
	}

	batch, err := repo.ListTaskRepositoriesByTaskIDs(ctx, []string{"task-pg-ordering-tiebreak"})
	if err != nil {
		t.Fatalf("ListTaskRepositoriesByTaskIDs: %v", err)
	}
	batchRows := batch["task-pg-ordering-tiebreak"]
	if len(batchRows) != 2 || batchRows[0].ID != "task-repository-pg-b" || batchRows[1].ID != "task-repository-pg-c" {
		t.Fatalf("ListTaskRepositoriesByTaskIDs expected id-ascending tiebreak [task-repository-pg-b task-repository-pg-c], got %+v", batchRows)
	}
}
