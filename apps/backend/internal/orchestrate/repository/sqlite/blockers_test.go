package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestTaskBlocker_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	blocker := &models.TaskBlocker{
		TaskID:        "task-1",
		BlockerTaskID: "task-2",
	}
	if err := repo.CreateTaskBlocker(ctx, blocker); err != nil {
		t.Fatalf("create: %v", err)
	}

	blockers, err := repo.ListTaskBlockers(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("count = %d, want 1", len(blockers))
	}
	if blockers[0].BlockerTaskID != "task-2" {
		t.Errorf("blocker_task_id = %q, want %q", blockers[0].BlockerTaskID, "task-2")
	}

	if err := repo.DeleteTaskBlocker(ctx, "task-1", "task-2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	blockers, _ = repo.ListTaskBlockers(ctx, "task-1")
	if len(blockers) != 0 {
		t.Errorf("count after delete = %d, want 0", len(blockers))
	}
}

func TestTaskBlocker_SelfReferenceBlocked(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	blocker := &models.TaskBlocker{
		TaskID:        "task-1",
		BlockerTaskID: "task-1",
	}
	if err := repo.CreateTaskBlocker(ctx, blocker); err == nil {
		t.Fatal("expected CHECK constraint error for self-reference")
	}
}
