package service

import (
	"context"
	"errors"
	"testing"
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

func TestProtectedTaskMetadataUpdatePreservesOnlyReservedKeys(t *testing.T) {
	updated := protectedTaskMetadataUpdate(
		map[string]interface{}{"b": 2, testBindingKey: "x"},
		map[string]interface{}{"a": 1},
	)
	if _, ok := updated["b"]; ok {
		t.Fatal("ordinary key must not be restored")
	}
	if updated[testBindingKey] != "x" {
		t.Fatal("reserved key must be restored")
	}
}

func TestTaskFieldUpdatePreservesCurrentCoordinatorBinding(t *testing.T) {
	services, _, gates := taskFieldServicePair(t)
	repo := gates[0].Repository
	seedFieldTask(t, repo, map[string]interface{}{testBindingKey: "original", "old": "removed"})
	requested := map[string]interface{}{"other": "value"}
	updated, err := runFieldInterleaving(t, services[0], gates[0], &UpdateTaskRequest{
		Metadata: requested,
	}, func(ctx context.Context) {
		if _, err := services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{
			Metadata:              map[string]interface{}{testBindingKey: "current"},
			AllowReservedMetadata: true,
		}); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetTask(context.Background(), "field-task")
	if err != nil {
		t.Fatal(err)
	}
	for _, metadata := range []map[string]interface{}{updated.Metadata, stored.Metadata} {
		if metadata[testBindingKey] != "current" || metadata["other"] != "value" {
			t.Fatalf("metadata replacement lost the current binding: %#v", metadata)
		}
		if _, present := metadata["old"]; present {
			t.Fatal("ordinary metadata must still be replaced")
		}
	}
	if _, present := requested[testBindingKey]; present {
		t.Fatal("metadata protection must not mutate the request")
	}
}
