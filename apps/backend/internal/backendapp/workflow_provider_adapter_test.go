package backendapp

import (
	"context"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Production workflowProviderAdapter must forward Prompt on UpdateWorkflow.
// Before this fix, import/sync assigned Prompt on the in-memory workflow then
// called UpdateWorkflow, which dropped it — so the DB never received the field.
func TestWorkflowProviderAdapter_UpdateWorkflowPersistsPrompt(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) == 0 {
		t.Fatal("expected seeded default workspace")
	}

	created, err := harness.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspaces[0].ID,
		Name:        "Adapter Prompt WF",
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	adapter := &workflowProviderAdapter{svc: harness.taskSvc}
	created.Prompt = "If the PR is merged or closed, move the Task to Done."
	if err := adapter.UpdateWorkflow(ctx, created); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	got, err := harness.taskSvc.GetWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Prompt != created.Prompt {
		t.Fatalf("Prompt = %q, want %q", got.Prompt, created.Prompt)
	}
	// Clear through the adapter must persist empty, not leave stale text.
	created.Prompt = ""
	if err := adapter.UpdateWorkflow(ctx, created); err != nil {
		t.Fatalf("UpdateWorkflow clear: %v", err)
	}
	got, err = harness.taskSvc.GetWorkflow(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkflow after clear: %v", err)
	}
	if got.Prompt != "" {
		t.Fatalf("Prompt after clear = %q, want empty", got.Prompt)
	}
}
