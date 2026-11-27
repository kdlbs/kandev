package coordinator

import (
	"context"
	"testing"
	"time"
)

func seedClaimProjectTables(t *testing.T, s *Store) {
	t.Helper()
	ts := "DATETIME"
	if s.db.DriverName() != "sqlite3" {
		ts = "TIMESTAMP"
	}
	mustExec(t, s, `CREATE TABLE IF NOT EXISTS repositories (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, deleted_at `+ts+`);
		CREATE TABLE IF NOT EXISTS repository_sets (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS repository_set_items (id TEXT PRIMARY KEY, repository_set_id TEXT NOT NULL, repository_id TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS task_repositories (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, repository_id TEXT NOT NULL);`)
}

func testWakeProjectClaimBoundary(t *testing.T, s *Store) {
	t.Helper()
	f := newWakeFixture(t, s)
	f.addOwn(t, "target", "workflow")
	seedClaimProjectTables(t, s)
	mustExec(t, s, `INSERT INTO repositories (id, workspace_id) VALUES ('repo', 'ws-1')`)
	mustExec(t, s, `INSERT INTO repository_sets (id, workspace_id) VALUES ('set', 'ws-1')`)
	mustExec(t, s, `INSERT INTO repository_set_items VALUES ('member', 'set', 'repo')`)
	mustExec(t, s, `INSERT INTO task_repositories VALUES ('link', 'target', 'repo')`)
	projectScope(t, s, f.c.ID, true, setEntry("set"))
	mustExec(t, s, `UPDATE coordinators SET cost_ceiling_subcents = 1000, conversation_task_id = 'conversation' WHERE id = ?`, f.c.ID)
	insertWakeRow(t, s, f.c, "wake", "target", string(WakeKindQuestion), "question", "pending", time.Now().UTC())
	watchSet, err := s.LoadWatchSet(t.Context(), s.ro, f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := turnStart{CoordinatorID: f.c.ID, ConvTaskID: "conversation", SessionID: "session", WakeIDs: []string{"wake"}, WatchSet: &watchSet}
	for _, change := range []string{
		`DELETE FROM repository_set_items`,
		`INSERT INTO repository_set_items VALUES ('member', 'set', 'repo'); UPDATE repositories SET workspace_id = 'foreign'`,
		`UPDATE repositories SET workspace_id = 'ws-1', deleted_at = CURRENT_TIMESTAMP`,
		`UPDATE repositories SET deleted_at = NULL; UPDATE repository_sets SET workspace_id = 'foreign'`,
	} {
		mustExec(t, s, change)
		started, err := s.startUnattendedTurn(context.Background(), in)
		if err != nil || started != nil {
			t.Fatalf("stale membership admitted after %q: %+v %v", change, started, err)
		}
		var status string
		if err := s.db.Get(&status, `SELECT status FROM coordinator_wakes WHERE id = 'wake'`); err != nil || status != "pending" {
			t.Fatalf("refused claim changed wake: %s %v", status, err)
		}
	}
	mustExec(t, s, `UPDATE repository_sets SET workspace_id = 'ws-1'`)
	started, err := s.startUnattendedTurn(t.Context(), in)
	if err != nil || started == nil || len(started.Wakes) != 1 {
		t.Fatalf("restored valid membership not admitted: %+v %v", started, err)
	}
}

func TestWakeProjectClaimBoundarySQLite(t *testing.T) {
	testWakeProjectClaimBoundary(t, newTestStore(t))
}
func TestWakeProjectClaimBoundaryPostgres(t *testing.T) {
	testWakeProjectClaimBoundary(t, newTestStorePostgres(t))
}
