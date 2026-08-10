package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestUpdateTaskMetadataPublishesMergedTaskUpdatedForPortForwarding(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-port", Name: "Port workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-port", WorkspaceID: "ws-port", Name: "Port workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-port",
		WorkspaceID:    "ws-port",
		WorkflowID:     "wf-port",
		WorkflowStepID: "step-port",
		Title:          "Port task",
		Priority:       "medium",
		Metadata:       map[string]interface{}{"unrelated": "preserve", models.MetaKeyPortForwardingEnabled: false},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	updated, err := svc.UpdateTaskMetadata(ctx, "task-port", map[string]interface{}{
		models.MetaKeyPortForwardingEnabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateTaskMetadata: %v", err)
	}
	if got := updated.Metadata[models.MetaKeyPortForwardingEnabled]; got != true {
		t.Fatalf("updated preference = %#v, want true", got)
	}
	if got := updated.Metadata["unrelated"]; got != "preserve" {
		t.Fatalf("updated unrelated metadata = %#v, want preserve", got)
	}

	persisted, err := repo.GetTask(ctx, "task-port")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got := persisted.Metadata[models.MetaKeyPortForwardingEnabled]; got != true {
		t.Fatalf("persisted preference = %#v, want true", got)
	}
	if got := persisted.Metadata["unrelated"]; got != "preserve" {
		t.Fatalf("persisted unrelated metadata = %#v, want preserve", got)
	}

	published := eventBus.GetPublishedEvents()
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	if published[0].Type != events.TaskUpdated {
		t.Fatalf("event type = %q, want %q", published[0].Type, events.TaskUpdated)
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", published[0].Data)
	}
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("event metadata type = %T, want map[string]interface{}", data["metadata"])
	}
	if got := metadata[models.MetaKeyPortForwardingEnabled]; got != true {
		t.Fatalf("event preference = %#v, want true", got)
	}
	if got := metadata["unrelated"]; got != "preserve" {
		t.Fatalf("event unrelated metadata = %#v, want preserve", got)
	}

	eventBus.ClearEvents()
	updated, err = svc.UpdateTaskMetadata(ctx, "task-port", map[string]interface{}{
		models.MetaKeyPortForwardingEnabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateTaskMetadata disable: %v", err)
	}
	if got := updated.Metadata[models.MetaKeyPortForwardingEnabled]; got != false {
		t.Fatalf("disabled preference = %#v, want false", got)
	}
	if got := updated.Metadata["unrelated"]; got != "preserve" {
		t.Fatalf("disabled unrelated metadata = %#v, want preserve", got)
	}
	if published := eventBus.GetPublishedEvents(); len(published) != 1 || published[0].Type != events.TaskUpdated {
		t.Fatalf("disabled update events = %#v, want one task.updated event", published)
	}
}
