package github

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestHandleTaskDeletedRemovesTaskPRsAndWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	for _, taskID := range []string{"deleted", "retained"} {
		seedTask(t, store, taskID, false)
		if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1}); err != nil {
			t.Fatalf("create task PR: %v", err)
		}
		mustCreateWatch(t, store, "session-"+taskID, taskID)
	}
	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{"task_id": "deleted"})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("repeat delivery: %v", err)
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "deleted"); len(got) != 0 {
		t.Fatalf("deleted task PRs = %d, want 0", len(got))
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "retained"); len(got) != 1 {
		t.Fatalf("retained task PRs = %d, want 1", len(got))
	}
	if got, _ := store.GetPRWatchBySession(ctx, "session-retained"); got == nil {
		t.Fatal("unrelated watch was removed")
	}
}
