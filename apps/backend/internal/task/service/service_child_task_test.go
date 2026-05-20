package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateChildTask_HappyPath_InheritsWorkflow(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childID, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:       "Child",
		Description: "Investigate",
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if childID == "" {
		t.Fatalf("expected non-empty child id")
	}

	got, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("parent_id = %q, want %q", got.ParentID, parent.ID)
	}
	if got.WorkflowID != parent.WorkflowID {
		t.Errorf("workflow_id = %q, want inherited %q", got.WorkflowID, parent.WorkflowID)
	}
	if got.WorkspaceID != parent.WorkspaceID {
		t.Errorf("workspace_id = %q, want %q", got.WorkspaceID, parent.WorkspaceID)
	}
	if got.Origin != models.TaskOriginAgentCreated {
		t.Errorf("origin = %q, want agent_created", got.Origin)
	}
}

func TestCreateChildTask_OverridesWorkflow(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Override with a workflow id different from parent's. The repository
	// stores whatever workflow id we pass — even if there's no rows for
	// it in workflow_steps, we just want to confirm the request shape.
	otherWorkflowID := parent.WorkflowID + "-override"
	childID, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:      "Switch flow",
		WorkflowID: otherWorkflowID,
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if childID == "" {
		t.Fatalf("expected non-empty child id")
	}
}

func TestCreateChildTask_RequiresParent(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	_, err := svc.CreateChildTask(context.Background(), nil, ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected parent error, got: %v", err)
	}
}

func TestCreateChildTask_RequiresTitle(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()
	parent, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	_, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title error, got: %v", err)
	}
}
