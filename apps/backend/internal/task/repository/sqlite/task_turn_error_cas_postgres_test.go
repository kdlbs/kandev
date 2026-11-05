package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// PostgreSQL runs stale full-task writes against real row locks and JSONB
// metadata updates. It skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresStaleFullTaskUpdatePreservesTurnErrorAttention(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID, sessionID = "task-turn-error-pg", "session-turn-error-pg"
	seedPostgresTaskSession(t, repo, taskID, sessionID)
	at := time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC)
	if _, err := repo.RecordTaskTurnError(ctx, taskID, models.TaskTurnError{
		StampValue: "pg-failure-1", SessionID: sessionID, MessageID: "pg-failure-1",
		OccurredAt: at, Message: "Internal error: Failed to execute statement",
	}); err != nil {
		t.Fatalf("record attention: %v", err)
	}
	stale, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load stale snapshot: %v", err)
	}
	stale.Metadata = nil
	stale.Title = "stale full write"
	if err := repo.UpdateTask(ctx, stale); err != nil {
		t.Fatalf("stale UpdateTask: %v", err)
	}
	current, ok := models.LoadTaskTurnError(mustTaskMetadata(t, repo, taskID, ctx))
	if !ok || current.StampValue != "pg-failure-1" || current.Acknowledged() {
		t.Fatalf("attention after stale write = %+v, want the live record", current)
	}

	staleAck, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}
	if _, err := repo.AcknowledgeTaskTurnErrorIfStamp(
		ctx, taskID, "pg-failure-1", at.Add(time.Minute), models.TurnErrorAcknowledgementDismiss,
	); err != nil {
		t.Fatalf("acknowledge attention: %v", err)
	}
	staleAck.Metadata = nil
	staleAck.Title = "stale deferred write"
	if err := repo.UpdateTaskPreservingDeferredLaunch(ctx, staleAck); err != nil {
		t.Fatalf("stale UpdateTaskPreservingDeferredLaunch: %v", err)
	}
	current, ok = models.LoadTaskTurnError(mustTaskMetadata(t, repo, taskID, ctx))
	if !ok || !current.Acknowledged() {
		t.Fatalf("acknowledged attention after stale write = %+v, want the acknowledgement preserved", current)
	}
}

func mustTaskMetadata(t *testing.T, repo *Repository, taskID string, ctx context.Context) map[string]interface{} {
	t.Helper()
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	return task.Metadata
}
