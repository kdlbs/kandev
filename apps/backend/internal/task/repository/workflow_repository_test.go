package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// Workflow CRUD tests

func TestSQLiteRepository_WorkflowCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workspace := &models.Workspace{ID: "ws-1", Name: "Workspace"}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create
	workflow := &models.Workflow{WorkspaceID: workspace.ID, Name: "Test Workflow", Description: "A test workflow"}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if workflow.ID == "" {
		t.Error("expected workflow ID to be set")
	}
	if workflow.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get
	retrieved, err := repo.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if retrieved.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", retrieved.Name)
	}

	// Update
	workflow.Name = "Updated Name"
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	retrieved, _ = repo.GetWorkflow(ctx, workflow.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %s", retrieved.Name)
	}

	// Delete
	if err := repo.DeleteWorkflow(ctx, workflow.ID); err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}
	_, err = repo.GetWorkflow(ctx, workflow.ID)
	if err == nil {
		t.Error("expected workflow to be deleted")
	}
}

func TestSQLiteRepository_WorkflowNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetWorkflow(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workflow")
	}

	err = repo.UpdateWorkflow(ctx, &models.Workflow{ID: "nonexistent", Name: "Test"})
	if err == nil {
		t.Error("expected error for updating nonexistent workflow")
	}

	err = repo.DeleteWorkflow(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent workflow")
	}
}

func TestSQLiteRepository_ListWorkflows(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})

	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow 1"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Workflow 2"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-3", WorkspaceID: "ws-2", Name: "Workflow 3"})

	workflows, err := repo.ListWorkflows(ctx, "ws-1")
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}
