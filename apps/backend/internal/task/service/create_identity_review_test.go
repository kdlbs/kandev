package service

import (
	"errors"
	"testing"
)

func TestCreateIdentityHitIgnoresUnusedReservedMetadata(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := t.Context()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "identity-review")
	first, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "identity-review", WorkflowID: wf,
		Title: "original", ExternalID: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "existing")
	bus.ClearEvents()
	metadata := map[string]interface{}{ReservedMetadataKeyPrefixCoordinator + "tool_policy": "untrusted"}
	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "identity-review", WorkflowID: wf,
		Title: "replacement", ExternalID: "existing", Metadata: metadata})
	if err != nil || retry.Outcome != CreateTaskOutcomeFoundSettled || retry.Task.ID != first.Task.ID {
		t.Fatalf("identity hit rejected unused create fields: %+v, %v", retry, err)
	}
	stored, err := repo.GetTask(ctx, first.Task.ID)
	if err != nil || stored.Title != "original" || stored.Metadata[ReservedMetadataKeyPrefixCoordinator+"tool_policy"] != nil || len(bus.GetPublishedEvents()) != 0 {
		t.Fatalf("identity hit mutated task: %+v, %v", stored, err)
	}
	_, err = svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "identity-review", WorkflowID: wf,
		Title: "new", ExternalID: "new", Metadata: metadata})
	if !errors.Is(err, ErrReservedMetadata) {
		t.Fatalf("new create accepted reserved metadata: %v", err)
	}
}
