package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// Task lifecycle cleanup is allowed to run before the optional message queue
// store is initialized. PostgreSQL aborts a transaction after a missing-table
// statement, so this verifies that DeleteTask leaves the transaction usable
// when queued_messages has not been created yet.
func TestPostgresDeleteTaskWithoutQueueSchema(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init task schema: %v", err)
	}
	ctx := context.Background()
	workspaceID := "workspace-missing-queue"
	taskID := "task-missing-queue"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask without queue schema: %v", err)
	}
}
