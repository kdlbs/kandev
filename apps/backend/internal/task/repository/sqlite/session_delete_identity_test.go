package sqlite

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
func deleteTaskSessionForTest(t *testing.T, repo *Repository, ctx context.Context, id string) error {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, id)
	if err != nil {
		return err
	}
	if session.QueueIncarnationID == "" {
		session.QueueIncarnationID = uuid.NewString()
		if _, err := repo.db.Exec(repo.db.Rebind(`
			UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?
		`), session.QueueIncarnationID, session.ID); err != nil {
			return err
		}
	}
	return repo.DeleteTaskSession(ctx, session)
}

func TestDeleteTaskSessionDoesNotDeleteReplacementIncarnation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-delete-identity"
		sessionID = "session-delete-identity"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Identity fence"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession initial: %v", err)
	}
	original, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession initial: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, original); err != nil {
		t.Fatalf("DeleteTaskSession initial: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession replacement: %v", err)
	}

	if err := repo.DeleteTaskSession(ctx, original); err == nil {
		t.Fatal("DeleteTaskSession delayed original succeeded")
	}
	if _, err := repo.GetTaskSession(ctx, sessionID); err != nil {
		t.Fatalf("replacement session was deleted: %v", err)
	}
}
