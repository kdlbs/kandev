package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const testBindingKey = ReservedMetadataKeyPrefixCoordinator + "tool_policy"

func TestCreateTaskRefusesReservedMetadataKey(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-meta-refuse")

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-meta-refuse",
		WorkflowID:  wfID,
		Title:       "Task",
		Metadata:    map[string]interface{}{testBindingKey: "forged"},
	})
	if !errors.Is(err, ErrReservedMetadata) {
		t.Fatalf("err = %v, want ErrReservedMetadata", err)
	}
}

func TestCreateTaskAllowsReservedMetadataKeyWhenFlagged(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-meta-allow")

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:           "ws-meta-allow",
		WorkflowID:            wfID,
		Title:                 "Task",
		Metadata:              map[string]interface{}{testBindingKey: "bound"},
		AllowReservedMetadata: true,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if got := result.Task.Metadata[testBindingKey]; got != "bound" {
		t.Fatalf("metadata = %v, want bound", got)
	}
}

func TestUpdateTaskReservedMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-meta-update")
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:           "ws-meta-update",
		WorkflowID:            wfID,
		Title:                 "Task",
		Metadata:              map[string]interface{}{testBindingKey: "bound"},
		AllowReservedMetadata: true,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	id := created.Task.ID

	if _, err := svc.UpdateTask(ctx, id, &UpdateTaskRequest{
		Metadata: map[string]interface{}{testBindingKey: "forged"},
	}); !errors.Is(err, ErrReservedMetadata) {
		t.Fatalf("forged update err = %v, want ErrReservedMetadata", err)
	}

	updated, err := svc.UpdateTask(ctx, id, &UpdateTaskRequest{
		Metadata: map[string]interface{}{"other": "value"},
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if got := updated.Metadata[testBindingKey]; got != "bound" {
		t.Fatalf("binding after unrelated update = %v, want bound", got)
	}

	rewritten, err := svc.UpdateTask(ctx, id, &UpdateTaskRequest{
		Metadata:              map[string]interface{}{testBindingKey: "rebound"},
		AllowReservedMetadata: true,
	})
	if err != nil {
		t.Fatalf("flagged UpdateTask: %v", err)
	}
	if got := rewritten.Metadata[testBindingKey]; got != "rebound" {
		t.Fatalf("binding after flagged update = %v, want rebound", got)
	}
}

func TestRestoreReservedMetadataIgnoresOrdinaryKeys(t *testing.T) {
	updated := map[string]interface{}{"a": 1}
	updated = models.ProtectedTaskMetadataUpdate(map[string]interface{}{"b": 2, testBindingKey: "x"}, updated)
	if _, ok := updated["b"]; ok {
		t.Fatal("ordinary key must not be restored")
	}
	if updated[testBindingKey] != "x" {
		t.Fatal("reserved key must be restored")
	}
}
