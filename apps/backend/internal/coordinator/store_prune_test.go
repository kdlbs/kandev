package coordinator

import (
	"context"
	"testing"
	"time"
)

// createMinimalTasksTable creates just enough of the tasks table for
// PruneStalls's NOT EXISTS join, mirroring internal/github's task-cleanup
// test convention of hand-creating a minimal tasks table in isolated package
// tests.
func createMinimalTasksTable(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
}

func seedTask(t *testing.T, store *Store, id string, archived bool) {
	t.Helper()
	var archivedAt interface{}
	if archived {
		archivedAt = time.Now().UTC()
	}
	if _, err := store.db.Exec(store.db.Rebind(`INSERT INTO tasks (id, archived_at) VALUES (?, ?)`), id, archivedAt); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

func insertStallRow(t *testing.T, store *Store, taskID, workspaceID string, detectedAt time.Time) {
	t.Helper()
	stall := sampleStall(taskID, workspaceID, time.Now().UTC())
	stall.DetectedAt = detectedAt
	if _, err := store.db.Exec(store.db.Rebind(`
		INSERT INTO coordinator_stalls (task_id, workspace_id, stalled_for_ms, last_event_at, detected_at)
		VALUES (?, ?, ?, ?, ?)`),
		stall.TaskID, stall.WorkspaceID, stall.StalledForMs, stall.LastEventAt, stall.DetectedAt); err != nil {
		t.Fatalf("insert stall %s: %v", taskID, err)
	}
}

func TestPruneStalls_DeletesMissingArchivedAndOldRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	createMinimalTasksTable(t, store)

	now := time.Now().UTC()
	seedTask(t, store, "task-old", false)
	seedTask(t, store, "task-archived", true)
	seedTask(t, store, "task-live", false)
	// "task-missing" is intentionally seeded with a stall but no tasks row.

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

func TestPruneStalls_NoTasksTablePrunesOnlyByAge(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertStallRow(t, store, "task-old", "ws-1", now.Add(-31*24*time.Hour))
	insertStallRow(t, store, "task-recent", "ws-1", now.Add(-time.Hour))

	deleted, err := store.PruneStalls(ctx, now)
	if err != nil {
		t.Fatalf("PruneStalls: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("PruneStalls deleted = %d, want 1", deleted)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 || list[0].TaskID != "task-recent" {
		t.Fatalf("ListStalls after prune = %+v, want only task-recent", list)
	}
}

func TestPruneStalls_NoRowsIsNoop(t *testing.T) {
	store := newTestStore(t)
	deleted, err := store.PruneStalls(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("PruneStalls: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("PruneStalls deleted = %d, want 0", deleted)
	}
}
