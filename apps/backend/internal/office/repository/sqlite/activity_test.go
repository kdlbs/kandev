package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestActivityEntry_CreateAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	entry := &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   "user",
		ActorID:     "user-1",
		Action:      "created_agent",
		TargetType:  "agent",
		TargetID:    "agent-1",
		Details:     `{"name":"test-agent"}`,
	}
	if err := repo.CreateActivityEntry(ctx, entry); err != nil {
		t.Fatalf("create: %v", err)
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("count = %d, want 1", len(entries))
	}
	if entries[0].Action != "created_agent" {
		t.Errorf("action = %q, want %q", entries[0].Action, "created_agent")
	}
}

// Pins that CreateActivityEntry round-trips the new run_id and
// session_id columns added in Wave 0. Without persistence here the
// run detail page's Tasks Touched surface (which reads back via
// ListTasksTouchedByRun) would silently miss agent-driven activity.
func TestActivityEntry_PersistsRunAndSessionID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	entry := &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   "agent",
		ActorID:     "agent-1",
		Action:      "task.commented",
		TargetType:  "task",
		TargetID:    "task-A",
		Details:     "{}",
		RunID:       "run-xyz",
		SessionID:   "sess-abc",
	}
	if err := repo.CreateActivityEntry(ctx, entry); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1", len(got))
	}
	if got[0].RunID != "run-xyz" {
		t.Errorf("run_id = %q, want run-xyz", got[0].RunID)
	}
	if got[0].SessionID != "sess-abc" {
		t.Errorf("session_id = %q, want sess-abc", got[0].SessionID)
	}

	// And it shows up in ListTasksTouchedByRun.
	tasks, err := repo.ListTasksTouchedByRun(ctx, "run-xyz")
	if err != nil {
		t.Fatalf("list tasks touched: %v", err)
	}
	if len(tasks) != 1 || tasks[0] != "task-A" {
		t.Fatalf("tasks touched = %v, want [task-A]", tasks)
	}
}

func TestActivityEntry_ListByTarget(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	entries := []*models.ActivityEntry{
		{
			WorkspaceID: "ws-1",
			ActorType:   "user",
			ActorID:     "user-1",
			Action:      "task.status_changed",
			TargetType:  "task",
			TargetID:    "task-1",
		},
		{
			WorkspaceID: "ws-1",
			ActorType:   "agent",
			ActorID:     "agent-1",
			Action:      "task.comment_created",
			TargetType:  "task",
			TargetID:    "task-2",
		},
	}
	for _, entry := range entries {
		if err := repo.CreateActivityEntry(ctx, entry); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := repo.ListActivityEntriesByTarget(ctx, "ws-1", "task-1", 10)
	if err != nil {
		t.Fatalf("list by target: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1", len(got))
	}
	if got[0].TargetID != "task-1" {
		t.Fatalf("target_id = %q, want task-1", got[0].TargetID)
	}
}
