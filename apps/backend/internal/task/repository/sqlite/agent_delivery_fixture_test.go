package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedAgentDeliveryTestSession(t *testing.T, repo *Repository, sessionID, incarnationID string) {
	t.Helper()
	ctx := context.Background()
	var exists bool
	if err := repo.db.QueryRowx(repo.db.Rebind(`SELECT EXISTS (SELECT 1 FROM task_sessions WHERE id = ?)`), sessionID).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`), incarnationID, sessionID); err != nil {
			t.Fatal(err)
		}
		return
	}
	taskID := "delivery-fixture-task-" + sessionID
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Delivery fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID}); err != nil {
		t.Fatal(err)
	}
}
