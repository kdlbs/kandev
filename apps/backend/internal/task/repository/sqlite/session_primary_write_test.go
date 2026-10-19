package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.1: a full-row update carrying a copy
// read while the session was primary must not make it primary again after a
// later promotion.
func TestUpdateTaskSessionKeepsPrimaryFromStaleCopy(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-stale-primary", Title: "Stale primary"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, id := range []string{"session-old-primary", "session-new-primary"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: id, TaskID: "task-stale-primary", State: models.TaskSessionStateRunning,
		}); err != nil {
			t.Fatalf("CreateTaskSession %s: %v", id, err)
		}
	}
	if err := repo.SetSessionPrimary(ctx, "session-old-primary"); err != nil {
		t.Fatalf("SetSessionPrimary old: %v", err)
	}
	stale, err := repo.GetTaskSession(ctx, "session-old-primary")
	if err != nil || !stale.IsPrimary {
		t.Fatalf("read primary copy: err=%v", err)
	}
	if err := repo.SetSessionPrimary(ctx, "session-new-primary"); err != nil {
		t.Fatalf("SetSessionPrimary new: %v", err)
	}

	stale.ErrorMessage = "unrelated write from a stale copy"
	if err := repo.UpdateTaskSession(ctx, stale); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	var primaries []string
	if err := repo.db.SelectContext(ctx, &primaries, repo.db.Rebind(
		`SELECT id FROM task_sessions WHERE task_id = ? AND is_primary = 1`,
	), "task-stale-primary"); err != nil {
		t.Fatalf("list primaries: %v", err)
	}
	if len(primaries) != 1 || primaries[0] != "session-new-primary" {
		t.Fatalf("primaries after stale full-row write = %v, want [session-new-primary]", primaries)
	}
	updated, err := repo.GetTaskSession(ctx, "session-old-primary")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if updated.ErrorMessage != "unrelated write from a stale copy" {
		t.Fatalf("full-row write lost its other columns: error_message=%q", updated.ErrorMessage)
	}
}
