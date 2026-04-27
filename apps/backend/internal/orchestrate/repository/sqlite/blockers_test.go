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

func TestGetChildSummaries(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// Create parent and children.
	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent Task", "", "")
	insertTask(t, repo, ctx, "child-1", "ws-1", "Auth service", "", "KAN-2")
	insertTask(t, repo, ctx, "child-2", "ws-1", "API gateway", "", "KAN-3")

	// Set parent_id and states.
	_, _ = repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'COMPLETED' WHERE id = 'child-1'`)
	_, _ = repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'CANCELLED' WHERE id = 'child-2'`)

	// Add a comment to child-1.
	_, _ = repo.ExecRaw(ctx,
		`INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		 VALUES ('c1', 'child-1', 'agent', 'a1', 'Implemented JWT generation', 'agent', datetime('now'))`)

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false for 2 children")
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// child-1 should have its comment.
	if summaries[0].Title != "Auth service" {
		t.Errorf("child-1 title = %q", summaries[0].Title)
	}
	if summaries[0].LastComment != "Implemented JWT generation" {
		t.Errorf("child-1 last_comment = %q", summaries[0].LastComment)
	}
	if summaries[0].State != "COMPLETED" {
		t.Errorf("child-1 state = %q", summaries[0].State)
	}

	// child-2 should have no comment.
	if summaries[1].LastComment != "" {
		t.Errorf("child-2 should have no comment, got %q", summaries[1].LastComment)
	}
}

func TestGetChildSummaries_NoChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "lonely-parent", "ws-1", "No Kids", "", "")

	summaries, truncated, err := repo.GetChildSummaries(ctx, "lonely-parent")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false")
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
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
