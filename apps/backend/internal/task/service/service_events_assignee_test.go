package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPublishTaskUpdated_EmitsHumanAssignee is the same gap as
// TestPublishTaskUpdated_EmitsAutoStartFailed, one field along: the payload
// map is hand-built, so a field the DTO carries is not automatically on the
// wire. Observed live before the fix: taking a task over left the previous
// owner's name in the top bar of every open client until a reload, because
// the event never mentioned the assignee and the frontend pins the value it
// already has when the key is absent.
func TestPublishTaskUpdated_EmitsHumanAssignee(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.PublishTaskUpdated(context.Background(), &models.Task{
		ID: "task-assignee", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		AssigneeUserID: "user-7",
	})

	data := singlePublishedEventData(t, eventBus)
	if got, _ := data["assignee_user_id"].(string); got != "user-7" {
		t.Fatalf("assignee_user_id payload = %#v, want user-7", data["assignee_user_id"])
	}
}

// TestPublishTaskUpdated_EmitsEmptyAssigneeWhenUnassigned proves unassigning
// propagates. An omitted key reads as "unchanged" on the frontend, so the
// empty string has to be explicit or clients would keep showing a name for a
// task nobody owns.
func TestPublishTaskUpdated_EmitsEmptyAssigneeWhenUnassigned(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.PublishTaskUpdated(context.Background(), &models.Task{
		ID: "task-unassigned", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	got, ok := data["assignee_user_id"]
	if !ok {
		t.Fatal("assignee_user_id absent from the payload: an unassign would be invisible to open clients")
	}
	if got != "" {
		t.Fatalf("assignee_user_id = %#v, want an explicit empty string", got)
	}
}
