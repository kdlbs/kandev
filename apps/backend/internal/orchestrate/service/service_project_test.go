package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestCreateProject_Valid(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:  "ws-1",
		Name:         "Test Project",
		Description:  "A test project",
		Repositories: `["https://github.com/team/backend"]`,
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}
	if project.ID == "" {
		t.Error("expected ID to be set")
	}
	if project.Status != models.ProjectStatusActive {
		t.Errorf("status = %q, want %q", project.Status, models.ProjectStatusActive)
	}
}

func TestCreateProject_EmptyName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "",
	}
	err := svc.CreateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateProject_EmptyWorkspaceID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		Name: "Test",
	}
	err := svc.CreateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for empty workspace ID")
	}
}

func TestCreateProject_InvalidStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "Test",
		Status:      "invalid_status",
	}
	err := svc.CreateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestCreateProject_InvalidRepositories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:  "ws-1",
		Name:         "Test",
		Repositories: `not-json`,
	}
	err := svc.CreateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for invalid repositories JSON")
	}
}

func TestCreateProject_EmptyRepoEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID:  "ws-1",
		Name:         "Test",
		Repositories: `["https://github.com/team/backend", ""]`,
	}
	err := svc.CreateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for empty repository entry")
	}
}

func TestUpdateProject_Validation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "Original",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	project.Name = ""
	err := svc.UpdateProject(ctx, project)
	if err == nil {
		t.Fatal("expected error for empty name on update")
	}
}

func TestListProjectsWithCounts_Service(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "Test Project",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	projects, err := svc.ListProjectsWithCounts(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list with counts: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("count = %d, want 1", len(projects))
	}
	if projects[0].TaskCounts.Total != 0 {
		t.Errorf("total = %d, want 0", projects[0].TaskCounts.Total)
	}
}

func TestGetTaskCounts_Service(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "Test Project",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	counts, err := svc.GetTaskCounts(ctx, project.ID)
	if err != nil {
		t.Fatalf("get task counts: %v", err)
	}
	if counts.Total != 0 {
		t.Errorf("total = %d, want 0", counts.Total)
	}
}
