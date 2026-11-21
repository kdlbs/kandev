package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

// TestCreateTaskAutomationStartOwnedRequiresSameWorkspaceParent pins the
// creation-time narrowing: an automation-started task re-validates its saved
// parent inside the task-creation admission, so a parent in another workspace
// is refused even when the automation's earlier parent check passed.
func TestCreateTaskAutomationStartOwnedRequiresSameWorkspaceParent(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	foreignWorkflow := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-automation-parent-a")
	ownWorkflow := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-automation-parent-b")

	foreign, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-automation-parent-a", WorkflowID: foreignWorkflow, Title: "Foreign parent",
	})
	if err != nil {
		t.Fatalf("create foreign parent: %v", err)
	}
	own, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-automation-parent-b", WorkflowID: ownWorkflow, Title: "Own parent",
	})
	if err != nil {
		t.Fatalf("create own parent: %v", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-automation-parent-b", WorkflowID: ownWorkflow, Title: "Automation child",
		ParentID: foreign.Task.ID, AutomationStartOwned: true,
	})
	if !errors.Is(err, hierarchy.ErrInvalidParent) {
		t.Fatalf("automation child under a foreign-workspace parent: err = %v, want ErrInvalidParent", err)
	}

	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-automation-parent-b", WorkflowID: ownWorkflow, Title: "Automation child",
		ParentID: own.Task.ID, AutomationStartOwned: true,
	})
	if err != nil {
		t.Fatalf("automation child under a same-workspace parent: %v", err)
	}
	if child.Task.ParentID != own.Task.ID {
		t.Fatalf("child parent = %q, want %q", child.Task.ParentID, own.Task.ID)
	}
}
