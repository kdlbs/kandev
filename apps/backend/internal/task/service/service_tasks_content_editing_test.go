package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// AC-29: committing a title on a task carrying a pending agent-generated
// title persists the user's title and clears the pending-agent-title
// metadata.
func TestUpdateTaskTitle_ClearsPendingAgentTitleMetadata(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-ac29", "sess-owner", true)

	newTitle := "Fix the login bug"
	task, err := svc.UpdateTask(ctx, "task-ac29", &UpdateTaskRequest{Title: &newTitle})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if task.Title != newTitle {
		t.Fatalf("title = %q, want %q", task.Title, newTitle)
	}
	if models.IsAgentTitlePending(task.Metadata) {
		t.Fatalf("metadata = %v, want the pending marker cleared", task.Metadata)
	}
	if models.AgentTitleOwnerSessionID(task.Metadata) != "" {
		t.Fatalf("metadata = %v, want the owner claim cleared", task.Metadata)
	}

	stored, err := repo.GetTask(ctx, "task-ac29")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != newTitle {
		t.Fatalf("persisted title = %q, want %q", stored.Title, newTitle)
	}
	if models.IsAgentTitlePending(stored.Metadata) {
		t.Fatalf("persisted metadata = %v, want the pending marker cleared", stored.Metadata)
	}

	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.TaskUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.TaskUpdated)
	}
}

// AC-48: committing a description-only save on a task carrying a pending
// agent-generated title persists the new description, leaves the
// pending-agent-title metadata intact, and leaves the stored title
// unchanged. This exercises the conditional title statement in
// updateTaskTx.
func TestUpdateTaskDescriptionOnly_LeavesPendingAgentTitleAndStoredTitleIntact(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-ac48", "sess-owner", true)

	newDescription := "Investigate the checkout flow regression"
	task, err := svc.UpdateTask(ctx, "task-ac48", &UpdateTaskRequest{Description: &newDescription})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if task.Description != newDescription {
		t.Fatalf("description = %q, want %q", task.Description, newDescription)
	}
	if task.Title != "Untitled" {
		t.Fatalf("title = %q, want the pending agent title left untouched", task.Title)
	}
	if !models.IsAgentTitlePending(task.Metadata) {
		t.Fatalf("metadata = %v, want the pending marker to survive a description-only save", task.Metadata)
	}
	if models.AgentTitleOwnerSessionID(task.Metadata) != "sess-owner" {
		t.Fatalf("metadata = %v, want the owner claim to survive a description-only save", task.Metadata)
	}

	stored, err := repo.GetTask(ctx, "task-ac48")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Description != newDescription {
		t.Fatalf("persisted description = %q, want %q", stored.Description, newDescription)
	}
	if stored.Title != "Untitled" {
		t.Fatalf("persisted title = %q, want it unchanged", stored.Title)
	}
	if !models.IsAgentTitlePending(stored.Metadata) {
		t.Fatalf("persisted metadata = %v, want the pending marker intact", stored.Metadata)
	}

	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.TaskUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.TaskUpdated)
	}
}

// AC-31: a title or description update publishes task.updated without
// assignee_agent_profile_id on the payload, so queueTaskAssignedRun cannot
// queue a run as a result of the edit.
func TestUpdateTaskTitleOrDescription_OmitsAssigneeAgentProfileIDFromPayload(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-ac31", WorkspaceID: "ws-title", WorkflowID: "wf-title", WorkflowStepID: "step-1",
		Title: "Original title", Description: "Original description", Priority: "medium",
		AssigneeAgentProfileID: "agent-x",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	bus.ClearEvents()

	newTitle := "Edited title"
	if _, err := svc.UpdateTask(ctx, "task-ac31", &UpdateTaskRequest{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateTask (title): %v", err)
	}
	titleData := singlePublishedEventData(t, bus)
	if _, ok := titleData["assignee_agent_profile_id"]; ok {
		t.Fatalf("title-only update published assignee_agent_profile_id = %#v, want the key absent",
			titleData["assignee_agent_profile_id"])
	}

	bus.ClearEvents()
	newDescription := "Edited description"
	if _, err := svc.UpdateTask(ctx, "task-ac31", &UpdateTaskRequest{Description: &newDescription}); err != nil {
		t.Fatalf("UpdateTask (description): %v", err)
	}
	descData := singlePublishedEventData(t, bus)
	if _, ok := descData["assignee_agent_profile_id"]; ok {
		t.Fatalf("description-only update published assignee_agent_profile_id = %#v, want the key absent",
			descData["assignee_agent_profile_id"])
	}
}
