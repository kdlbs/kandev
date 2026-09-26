package usage

// End-to-end coverage through the real bus -> Writer -> *sqlite.Repository
// path (docs/specs/task-cost-ledger/spec.md AC-16, AC-34), as opposed to the
// fakeUsageRepo test double used everywhere else in this package.

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestWriter_EndToEnd_RealRepoPreservesArrivalOrder pins AC-16 (occurred_at
// ascending, id as tiebreak) together with AC-34 (the bus callback hands off
// to a single serial worker, off the publisher's own goroutine). occurred_at
// is acquired by the worker at processing time (buildRow), not carried on
// the payload, so fast back-to-back events can legitimately collide on that
// column; proving arrival order survives end to end through a real
// repository also proves the id tiebreak is doing its job, not just that
// the column exists in isolation.
func TestWriter_EndToEnd_RealRepoPreservesArrivalOrder(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "writer-e2e.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqliterepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "task-e2e", WorkspaceID: "ws-1", Title: "e2e", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "session-e2e", TaskID: "task-e2e", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	w := NewWriter(repo, nil, nil)
	w.Start()

	eb := bus.NewMemoryEventBus(logger.Default())
	if err := w.Subscribe(eb); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	subject := events.BuildSessionPromptUsageSubject("session-e2e")
	const n = 8
	for i := 0; i < n; i++ {
		payload := map[string]any{
			"task_id":        "task-e2e",
			"session_id":     "session-e2e",
			"usage_event_id": fmt.Sprintf("evt-e2e-%d", i),
			"usage":          map[string]any{"input_tokens": int64(i)},
		}
		if err := eb.Publish(context.Background(), subject, &bus.Event{Data: payload}); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	w.Stop()

	got, err := repo.ListTaskUsageEvents(context.Background(), "task-e2e", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(got) != n {
		t.Fatalf("row count = %d, want %d", len(got), n)
	}
	for i, row := range got {
		want := fmt.Sprintf("evt-e2e-%d", i)
		if row.UsageEventID != want {
			t.Errorf("row %d usage_event_id = %q, want %q (arrival order must survive the occurred_at+id ordering)", i, row.UsageEventID, want)
		}
	}
}
