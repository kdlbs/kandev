package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPublishTaskUpdated_IncludesExternalIDWhenHeld covers the spec's
// requirement that external_id joins the hand-built WS event map (it is not
// derived from TaskDTO, so needs its own explicit field).
func TestPublishTaskUpdated_IncludesExternalIDWhenHeld(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "T", Priority: "medium", ExternalID: "ext-1",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", ExternalID: "ext-1"}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if got, ok := data["external_id"].(string); !ok || got != "ext-1" {
		t.Fatalf("external_id = %#v, want \"ext-1\"", data["external_id"])
	}
}

// TestPublishTaskUpdated_OmitsExternalIDWhenAbsent covers the "omitted
// rather than null/empty-string" requirement for a task holding no identity.
func TestPublishTaskUpdated_OmitsExternalIDWhenAbsent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-2", WorkspaceID: "ws-2", WorkflowID: "wf-2", Title: "T", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	task := &models.Task{ID: "task-2", WorkspaceID: "ws-2", WorkflowID: "wf-2"}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if _, present := data["external_id"]; present {
		t.Fatalf("external_id should be omitted entirely for a task holding no identity, got %#v", data["external_id"])
	}
}
