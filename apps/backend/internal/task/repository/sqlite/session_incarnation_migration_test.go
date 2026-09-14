package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
func TestTaskSessionQueueIncarnationIsGeneratedProjectedAndRenewed(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-queue-incarnation"
		sessionID = "session-reused"
	)

	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Queue incarnation"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession first: %v", err)
	}

	var first string
	if err := repo.db.GetContext(ctx, &first, `SELECT queue_incarnation_id FROM task_sessions WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("read first queue incarnation: %v", err)
	}
	if first == "" {
		t.Fatal("first queue incarnation is empty")
	}
	stored, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession first: %v", err)
	}
	if got := stored.ToAPI()["queue_incarnation_id"]; got != first {
		t.Fatalf("projected queue incarnation = %v, want %q", got, first)
	}

	if err := repo.DeleteTaskSession(ctx, stored); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession replacement: %v", err)
	}
	var replacement string
	if err := repo.db.GetContext(ctx, &replacement, `SELECT queue_incarnation_id FROM task_sessions WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("read replacement queue incarnation: %v", err)
	}
	if replacement == "" || replacement == first {
		t.Fatalf("replacement queue incarnation = %q, want non-empty value different from %q", replacement, first)
	}
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
func TestTaskSessionQueueIncarnationMigrationBackfillsDistinctValuesOnReplay(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-incarnation-backfill", Name: "Backfill"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-incarnation-backfill", WorkspaceID: "workspace-incarnation-backfill", Title: "Backfill",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, sessionID := range []string{"session-backfill-a", "session-backfill-b"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: "task-incarnation-backfill"}); err != nil {
			t.Fatalf("CreateTaskSession %s: %v", sessionID, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_sessions SET queue_incarnation_id = ''`); err != nil {
		t.Fatalf("clear queue incarnations: %v", err)
	}

	if _, err := NewWithDB(repo.db, repo.ro, nil); err != nil {
		t.Fatalf("replay repository migrations: %v", err)
	}
	var incarnations []string
	if err := repo.db.SelectContext(ctx, &incarnations, `SELECT queue_incarnation_id FROM task_sessions ORDER BY id`); err != nil {
		t.Fatalf("list queue incarnations: %v", err)
	}
	if len(incarnations) != 2 || incarnations[0] == "" || incarnations[1] == "" || incarnations[0] == incarnations[1] {
		t.Fatalf("backfilled queue incarnations = %#v, want two distinct non-empty values", incarnations)
	}
}
