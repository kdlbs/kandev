package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
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
