package sqlite

// Postgres parity coverage for the session reconciliation sweep's reads.
// GetLastMessageTimeBySessionIDs aggregates MAX(updated_at) over
// task_session_messages and scans the result per dialect, and
// ListUnarchivedTasksWithActiveSessions projects full task rows through a
// DISTINCT join. The SQLite-only run never exercises PostgreSQL's own
// timestamp and DISTINCT-interaction behavior on these paths.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresSessionSweepReads(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-sweep-pg")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-sweep-pg", TaskID: "task-sweep-pg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	oldest := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Microsecond)
	newest := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-sweep-pg", TaskSessionID: "session-sweep-pg", TaskID: "task-sweep-pg", StartedAt: oldest,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	for _, msg := range []*models.Message{
		{ID: "msg-sweep-pg-old", TaskSessionID: "session-sweep-pg", TaskID: "task-sweep-pg", TurnID: "turn-sweep-pg", Content: "old", UpdatedAt: oldest},
		{ID: "msg-sweep-pg-new", TaskSessionID: "session-sweep-pg", TaskID: "task-sweep-pg", TurnID: "turn-sweep-pg", Content: "new", UpdatedAt: newest},
	} {
		if err := repo.CreateMessage(ctx, msg); err != nil {
			t.Fatalf("CreateMessage(%s): %v", msg.ID, err)
		}
	}

	tasks, err := repo.ListUnarchivedTasksWithActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListUnarchivedTasksWithActiveSessions: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.ID == "task-sweep-pg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("task-sweep-pg not returned among %d candidates", len(tasks))
	}

	lastMessage, err := repo.GetLastMessageTimeBySessionIDs(ctx, []string{"session-sweep-pg"})
	if err != nil {
		t.Fatalf("GetLastMessageTimeBySessionIDs: %v", err)
	}
	if got, ok := lastMessage["session-sweep-pg"]; !ok || !got.Equal(newest) {
		t.Fatalf("last message time = %v (present=%v), want the newest message time %v", got, ok, newest)
	}
}
