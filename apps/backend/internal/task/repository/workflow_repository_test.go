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

	workflows, err := repo.ListWorkflows(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}

func TestSQLiteRepository_WorkflowSortOrder(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	// Create workflows and verify auto-assigned sort_order
	wf1 := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "First"}
	wf2 := &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Second"}
	wf3 := &models.Workflow{ID: "wf-3", WorkspaceID: "ws-1", Name: "Third"}
	_ = repo.CreateWorkflow(ctx, wf1)
	_ = repo.CreateWorkflow(ctx, wf2)
	_ = repo.CreateWorkflow(ctx, wf3)

	if wf1.SortOrder != 0 {
		t.Errorf("expected first workflow sort_order=0, got %d", wf1.SortOrder)
	}
	if wf2.SortOrder != 1 {
		t.Errorf("expected second workflow sort_order=1, got %d", wf2.SortOrder)
	}
	if wf3.SortOrder != 2 {
		t.Errorf("expected third workflow sort_order=2, got %d", wf3.SortOrder)
	}

	// Verify sort_order is persisted via Get
	retrieved, _ := repo.GetWorkflow(ctx, "wf-2")
	if retrieved.SortOrder != 1 {
		t.Errorf("expected retrieved sort_order=1, got %d", retrieved.SortOrder)
	}

	// Verify ListWorkflows returns in sort_order
	workflows, _ := repo.ListWorkflows(ctx, "ws-1", false)
	if len(workflows) != 3 {
		t.Fatalf("expected 3 workflows, got %d", len(workflows))
	}
	if workflows[0].Name != "First" || workflows[1].Name != "Second" || workflows[2].Name != "Third" {
		t.Errorf("expected workflows in sort_order, got: %s, %s, %s",
			workflows[0].Name, workflows[1].Name, workflows[2].Name)
	}
}

func TestSQLiteRepository_ReorderWorkflows(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "First"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Second"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-3", WorkspaceID: "ws-1", Name: "Third"})

	// Reorder: Third, First, Second
	err := repo.ReorderWorkflows(ctx, "ws-1", []string{"wf-3", "wf-1", "wf-2"})
	if err != nil {
		t.Fatalf("failed to reorder workflows: %v", err)
	}

	workflows, _ := repo.ListWorkflows(ctx, "ws-1", false)
	if len(workflows) != 3 {
		t.Fatalf("expected 3 workflows, got %d", len(workflows))
	}
	if workflows[0].Name != "Third" || workflows[1].Name != "First" || workflows[2].Name != "Second" {
		t.Errorf("expected reordered workflows [Third, First, Second], got: [%s, %s, %s]",
			workflows[0].Name, workflows[1].Name, workflows[2].Name)
	}

	// Verify individual sort_order values
	wf3, _ := repo.GetWorkflow(ctx, "wf-3")
	wf1, _ := repo.GetWorkflow(ctx, "wf-1")
	wf2, _ := repo.GetWorkflow(ctx, "wf-2")
	if wf3.SortOrder != 0 {
		t.Errorf("expected wf-3 sort_order=0, got %d", wf3.SortOrder)
	}
	if wf1.SortOrder != 1 {
		t.Errorf("expected wf-1 sort_order=1, got %d", wf1.SortOrder)
	}
	if wf2.SortOrder != 2 {
		t.Errorf("expected wf-2 sort_order=2, got %d", wf2.SortOrder)
	}
}

func TestSQLiteRepository_ReorderWorkflows_InvalidID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "First"})

	err := repo.ReorderWorkflows(ctx, "ws-1", []string{"wf-1", "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent workflow ID in reorder")
	}
}

func TestSQLiteRepository_ReorderWorkflows_WrongWorkspace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "First"})

	// Try to reorder workflow from ws-1 in ws-2 — should fail
	err := repo.ReorderWorkflows(ctx, "ws-2", []string{"wf-1"})
	if err == nil {
		t.Error("expected error when reordering workflow from wrong workspace")
	}
}

func TestSQLiteRepository_WorkflowAgentProfileID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	// Create workflow with agent_profile_id
	workflow := &models.Workflow{
		WorkspaceID:    "ws-1",
		Name:           "Profile Workflow",
		AgentProfileID: "profile-123",
	}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Get and verify agent_profile_id persists
	retrieved, err := repo.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if retrieved.AgentProfileID != "profile-123" {
		t.Errorf("expected agent_profile_id 'profile-123', got %q", retrieved.AgentProfileID)
	}

	// Update agent_profile_id
	workflow.AgentProfileID = "profile-456"
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	retrieved, _ = repo.GetWorkflow(ctx, workflow.ID)
	if retrieved.AgentProfileID != "profile-456" {
		t.Errorf("expected agent_profile_id 'profile-456', got %q", retrieved.AgentProfileID)
	}

	// Clear agent_profile_id
	workflow.AgentProfileID = ""
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	retrieved, _ = repo.GetWorkflow(ctx, workflow.ID)
	if retrieved.AgentProfileID != "" {
		t.Errorf("expected empty agent_profile_id, got %q", retrieved.AgentProfileID)
	}

	// Verify ListWorkflows includes agent_profile_id
	_ = repo.CreateWorkflow(ctx, &models.Workflow{
		WorkspaceID:    "ws-1",
		Name:           "Another Workflow",
		AgentProfileID: "profile-789",
	})
	workflows, err := repo.ListWorkflows(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	found := false
	for _, wf := range workflows {
		if wf.AgentProfileID == "profile-789" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find workflow with agent_profile_id 'profile-789' in list")
	}
}

func TestSQLiteRepository_WorkflowSortOrder_Isolation(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})

	// Create workflows in different workspaces — sort_order should be independent
	wf1 := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WS1 First"}
	wf2 := &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2", Name: "WS2 First"}
	wf3 := &models.Workflow{ID: "wf-3", WorkspaceID: "ws-1", Name: "WS1 Second"}
	_ = repo.CreateWorkflow(ctx, wf1)
	_ = repo.CreateWorkflow(ctx, wf2)
	_ = repo.CreateWorkflow(ctx, wf3)

	if wf1.SortOrder != 0 {
		t.Errorf("expected ws-1 first workflow sort_order=0, got %d", wf1.SortOrder)
	}
	if wf2.SortOrder != 0 {
		t.Errorf("expected ws-2 first workflow sort_order=0, got %d", wf2.SortOrder)
	}
	if wf3.SortOrder != 1 {
		t.Errorf("expected ws-1 second workflow sort_order=1, got %d", wf3.SortOrder)
	}
}
