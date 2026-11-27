package coordinator

import (
	"context"
	"testing"
	"time"
)

func createMinimalTasksTablePostgres(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at TIMESTAMPTZ)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
}

// TestPruneStalls_Postgres_DeletesMissingArchivedAndOldRows is the PostgreSQL
// twin of TestPruneStalls_DeletesMissingArchivedAndOldRows.
func TestPruneStalls_Postgres_DeletesMissingArchivedAndOldRows(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	createMinimalTasksTablePostgres(t, store)

	now := time.Now().UTC()
	seedTask(t, store, "task-old", false)
	seedTask(t, store, "task-archived", true)
	seedTask(t, store, "task-live", false)

	insertStallRow(t, store, "task-old", "ws-1", now.Add(-31*24*time.Hour))
	insertStallRow(t, store, "task-archived", "ws-1", now.Add(-time.Hour))
	insertStallRow(t, store, "task-missing", "ws-1", now.Add(-time.Hour))
	insertStallRow(t, store, "task-live", "ws-1", now.Add(-time.Hour))

	deleted, err := store.PruneStalls(ctx, now)
	if err != nil {
		t.Fatalf("PruneStalls: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("PruneStalls deleted = %d, want 3", deleted)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 || list[0].TaskID != "task-live" {
		t.Fatalf("ListStalls after prune = %+v, want only task-live", list)
	}
}
