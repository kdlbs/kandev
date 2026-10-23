package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
)

func TestMCPAssignmentOwnerRejectsArchivedTask(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("workspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, false)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("workflows: %v", err)
	}
	now := time.Now().UTC()
	task := &models.Task{ID: "mcp-archived-owner", Title: "Archived", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, ArchivedAt: &now}
	if err := harness.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := harness.taskRepo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	validator := mcpSelectionAdditionValidator(routeParams{taskSvc: harness.taskSvc})
	if err := validator(ctx, mcpconfig.SelectionScopeTask, task.WorkspaceID, task.ID); err == nil {
		t.Fatal("archived owner is eligible")
	}
}
