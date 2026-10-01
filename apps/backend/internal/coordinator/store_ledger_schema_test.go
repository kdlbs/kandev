package coordinator

import (
	"context"
	"testing"
	"time"
)

func seedLedgerRows(t *testing.T, s *Store, c *Coordinator) {
	t.Helper()
	now := time.Now().UTC()
	mustExec(t, s, `INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at)
		VALUES ('t1', ?, 's1', 'st1', 'message', ?)`, c.ID, now)
	mustExec(t, s, `INSERT INTO coordinator_turn_calls (turn_id, action, allowed, recorded_at) VALUES ('t1', 'a', ?, ?)`, true, now)
	mustExec(t, s, `INSERT INTO coordinator_turn_snapshots (hash, body, created_at) VALUES ('h1', '{}', ?)`, now)
}

func ledgerSchemaChecks(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	mustExec(t, s, `INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at)
		VALUES ('p1', ?, 'ws-1', 'pending', '{}', ?, ?)`, c.ID, time.Now().UTC(), time.Now().UTC())
	seedLedgerRows(t, s, c)

	var turnID *string
	if err := s.db.QueryRow(`SELECT turn_id FROM coordinator_proposals WHERE id = 'p1'`).Scan(&turnID); err != nil || turnID != nil {
		t.Fatalf("existing proposal turn_id = %v, err %v; want NULL", turnID, err)
	}
	// A second ledger row for the same session turn is refused.
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at)
		VALUES ('t2', ?, 's1', 'st1', 'message', ?)`), c.ID, time.Now().UTC()); err == nil {
		t.Fatal("duplicate (session_id, session_turn_id) accepted")
	}
	// A ledger row links to at most one unattended-turn row.
	for _, id := range []string{"u1", "u2"} {
		mustExec(t, s, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at)
			VALUES (?, ?, 'conv', 's1', 1, 100, 'completed', ?)`, id, c.ID, time.Now().UTC())
	}
	mustExec(t, s, `UPDATE coordinator_unattended_turns SET ledger_turn_id = 't1' WHERE id = 'u1'`)
	if _, err := s.db.Exec(s.db.Rebind(`UPDATE coordinator_unattended_turns SET ledger_turn_id = 't1' WHERE id = 'u2'`)); err == nil {
		t.Fatal("second unattended row linked to the same ledger row")
	}
	if _, err := NewStore(s.db, s.ro); err != nil {
		t.Fatalf("replay NewStore: %v", err)
	}
	if err := s.DeleteCoordinator(ctx, c.WorkspaceID, c.ID); err != nil {
		t.Fatalf("DeleteCoordinator: %v", err)
	}
	if n := count(t, s, "coordinator_turns") + count(t, s, "coordinator_turn_calls"); n != 0 {
		t.Fatalf("ledger rows left after coordinator delete: %d", n)
	}
	if n := count(t, s, "coordinator_turn_snapshots"); n != 1 {
		t.Fatalf("snapshots = %d, want 1 (shared by hash, left to age out)", n)
	}
}

func TestLedgerSchema(t *testing.T) { ledgerSchemaChecks(t, newTestStore(t)) }

func TestLedgerSchema_Postgres(t *testing.T) { ledgerSchemaChecks(t, newTestStorePostgres(t)) }

func dropLedgerSchema(t *testing.T, s *Store) {
	t.Helper()
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_coordinator_proposals_turn`,
		`DROP INDEX IF EXISTS idx_coordinator_activity_turn`,
		`DROP INDEX IF EXISTS idx_coordinator_unattended_turns_ledger`,
		`ALTER TABLE coordinator_proposals DROP COLUMN turn_id`,
		`ALTER TABLE coordinator_activity DROP COLUMN turn_id`,
		`ALTER TABLE coordinator_unattended_turns DROP COLUMN ledger_turn_id`,
		`DROP TABLE coordinator_turn_calls`,
		`DROP TABLE coordinator_turn_snapshots`,
		`DROP TABLE coordinator_turns`,
	} {
		mustExec(t, s, stmt)
	}
}

func ledgerUpgradeChecks(t *testing.T, s *Store) {
	t.Helper()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	dropLedgerSchema(t, s)
	now := time.Now().UTC()
	mustExec(t, s, `INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at)
		VALUES ('p1', ?, 'ws-1', 'pending', '{}', ?, ?)`, c.ID, now, now)
	mustExec(t, s, `INSERT INTO coordinator_activity (id, coordinator_id, workspace_id, action_class, outcome, "authorization", created_at, updated_at)
		VALUES ('a1', ?, 'ws-1', 'move', 'proposed', 'denied', ?, ?)`, c.ID, now, now)
	mustExec(t, s, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at)
		VALUES ('u1', ?, 'conv', 's1', 1, 100, 'completed', ?)`, c.ID, now)

	if _, err := NewStore(s.db, s.ro); err != nil {
		t.Fatalf("upgrade NewStore: %v", err)
	}
	for table, want := range map[string]int{"coordinator_proposals": 1, "coordinator_activity": 1, "coordinator_unattended_turns": 1, "coordinator_turns": 0} {
		if got := count(t, s, table); got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
	for _, q := range []string{
		`SELECT turn_id FROM coordinator_proposals WHERE id = 'p1'`,
		`SELECT turn_id FROM coordinator_activity WHERE id = 'a1'`,
		`SELECT ledger_turn_id FROM coordinator_unattended_turns WHERE id = 'u1'`,
	} {
		var link *string
		if err := s.db.QueryRow(q).Scan(&link); err != nil || link != nil {
			t.Fatalf("%s = %v, err %v; want NULL", q, link, err)
		}
	}
}

func TestLedgerUpgradeKeepsEveryExistingRow(t *testing.T) { ledgerUpgradeChecks(t, newTestStore(t)) }

func TestLedgerUpgradeKeepsEveryExistingRow_Postgres(t *testing.T) {
	ledgerUpgradeChecks(t, newTestStorePostgres(t))
}

func TestDeleteWorkspaceStateRemovesLedgerRowsAndKeepsSharedSnapshots(t *testing.T) {
	s := newTestStore(t)
	c := &Coordinator{WorkspaceID: "ws-1", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	seedLedgerRows(t, s, c)
	if err := s.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceState: %v", err)
	}
	if n := count(t, s, "coordinator_turns") + count(t, s, "coordinator_turn_calls"); n != 0 {
		t.Fatalf("ledger rows left after workspace delete: %d", n)
	}
	if n := count(t, s, "coordinator_turn_snapshots"); n != 1 {
		t.Fatalf("snapshots = %d, want 1", n)
	}
}
