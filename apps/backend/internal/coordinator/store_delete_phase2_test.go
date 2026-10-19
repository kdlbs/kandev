package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"
)

var phase2Tables = []string{"coordinator_watches", "coordinator_watch_projects", "coordinator_activity", "coordinator_standing_orders", "coordinator_goals"}

func seedPhase2Rows(t *testing.T, store *Store, c *Coordinator) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	q := func(query string, args ...any) {
		t.Helper()
		if _, err := store.db.ExecContext(ctx, store.db.Rebind(query), args...); err != nil {
			t.Fatal(err)
		}
	}
	q(`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf', ?, ?)`, c.ID, c.WorkspaceID, now)
	q(`INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, 'repository', 'repo', ?, ?)`, c.ID, c.WorkspaceID, now)
	q(`INSERT INTO coordinator_standing_orders (id, coordinator_id, workspace_id, text, created_by, created_at) VALUES ('so-'||?, ?, ?, 't', 'u', ?)`, c.ID, c.ID, c.WorkspaceID, now)
	q(`INSERT INTO coordinator_goals (id, coordinator_id, workspace_id, name, status, baseline_json, set_at, created_at, updated_at) VALUES ('g-'||?, ?, ?, 'n', 'active', '{}', ?, ?, ?)`, c.ID, c.ID, c.WorkspaceID, now, now, now)
	row := validRow(c.ID)
	row.WorkspaceID = c.WorkspaceID
	if err := store.InsertActivity(ctx, store.db, row); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, store *Store, table string) int {
	t.Helper()
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM `+table); err != nil {
		t.Fatal(err)
	}
	return n
}

func runDeleteCleansPhase2(t *testing.T, store *Store) {
	ctx := context.Background()
	a := newTestCoordinator(t, store, "ws-del")
	b := newTestCoordinator(t, store, "ws-keep")
	seedPhase2Rows(t, store, a)
	seedPhase2Rows(t, store, b)
	if err := store.DeleteCoordinator(ctx, "ws-del", a.ID); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range phase2Tables {
		if n := countRows(t, store, tbl); n != 1 {
			t.Fatalf("%s rows after coordinator delete = %d, want 1", tbl, n)
		}
	}
	if err := store.DeleteWorkspaceState(ctx, "ws-keep"); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range phase2Tables {
		if n := countRows(t, store, tbl); n != 0 {
			t.Fatalf("%s rows after workspace delete = %d", tbl, n)
		}
	}
}

func runRecordVsWorkspaceDelete(t *testing.T, store *Store) {
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-race")
	svc := newPhase2Service(t, store, true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		row := validRow(c.ID)
		row.WorkspaceID = "ws-race"
		_ = store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return svc.Record(ctx, tx, row) })
	}()
	go func() {
		defer wg.Done()
		_ = store.DeleteWorkspaceState(ctx, "ws-race")
	}()
	wg.Wait()
	for _, tbl := range phase2Tables {
		if n := countRows(t, store, tbl); n != 0 {
			t.Fatalf("%s rows survive workspace delete = %d", tbl, n)
		}
	}
}

func TestDelete_RemovesPhase2Rows_SQLite(t *testing.T) { runDeleteCleansPhase2(t, newTestStore(t)) }
func TestDelete_RemovesPhase2Rows_Postgres(t *testing.T) {
	runDeleteCleansPhase2(t, newTestStorePostgres(t))
}
func TestRecordVsWorkspaceDelete_SQLite(t *testing.T) { runRecordVsWorkspaceDelete(t, newTestStore(t)) }
func TestRecordVsWorkspaceDelete_Postgres(t *testing.T) {
	runRecordVsWorkspaceDelete(t, newTestStorePostgres(t))
}
