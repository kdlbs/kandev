package service

import (
	"context"
	"testing"
	"time"
)

func TestSourceIdentityDedup(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-source-dedup")
	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-source-dedup", WorkflowID: wfID, Title: "Linked task", ExternalID: "coordinator-proposal-1",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "coordinator-proposal-1")
	if _, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET archived_at = ? WHERE id = ?", time.Now().UTC(), first.Task.ID); err != nil {
		t.Fatalf("archive source task: %v", err)
	}

	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-source-dedup", WorkflowID: wfID, Title: "Retry must not create another task", ExternalID: "coordinator-proposal-1",
	})
	if err != nil {
		t.Fatalf("retry archived source identity: %v", err)
	}
	if retry.Outcome != CreateTaskOutcomeFoundSettled || retry.Task.ID != first.Task.ID || retry.Task.ArchivedAt == nil {
		t.Fatalf("archived source identity result = %+v, want original archived task", retry)
	}
	createdEvents := 0
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == "task.created" {
			createdEvents++
		}
	}
	if createdEvents != 1 {
		t.Fatalf("task.created events = %d, want one across retry", createdEvents)
	}
}
