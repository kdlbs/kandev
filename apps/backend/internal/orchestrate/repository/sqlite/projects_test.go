package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

func TestProject_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "Test Project",
		Description:    "A test project",
		Status:         models.ProjectStatusActive,
		BudgetCents:    10000,
		Repositories:   "[]",
		ExecutorConfig: "{}",
	}
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Test Project" {
		t.Errorf("name = %q, want %q", got.Name, "Test Project")
	}

	projects, err := repo.ListProjects(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("list count = %d, want 1", len(projects))
	}

	project.Status = models.ProjectStatusCompleted
	if err := repo.UpdateProject(ctx, project); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.GetProject(ctx, project.ID)
	if got.Status != models.ProjectStatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, models.ProjectStatusCompleted)
	}

	if err := repo.DeleteProject(ctx, project.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func newTestRepoWithTasks(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		project_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		title TEXT DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func TestProject_GetTaskCounts(t *testing.T) {
	repo := newTestRepoWithTasks(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "Task Count Test",
		Status:         models.ProjectStatusActive,
		Repositories:   "[]",
		ExecutorConfig: "{}",
	}
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	counts, err := repo.GetTaskCounts(ctx, project.ID)
	if err != nil {
		t.Fatalf("get task counts: %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("total = %d, want 0", counts.Total)
	}
}

func TestProject_ListWithCounts(t *testing.T) {
	repo := newTestRepoWithTasks(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "List Test",
		Status:         models.ProjectStatusActive,
		Repositories:   "[]",
		ExecutorConfig: "{}",
	}
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	results, err := repo.ListProjectsWithCounts(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list with counts: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("count = %d, want 1", len(results))
	}
	if results[0].Name != "List Test" {
		t.Errorf("name = %q, want %q", results[0].Name, "List Test")
	}
}
