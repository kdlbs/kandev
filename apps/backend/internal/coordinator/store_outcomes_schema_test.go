package coordinator

import (
	"context"
	"testing"
	"time"
)

func seedOutcomeRows(t *testing.T, s *Store, coordinatorID string, at time.Time) {
	t.Helper()
	mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, kind, decision, decided_at, graded_at)
		VALUES ('p-out', ?, 'create_task', 'approved', ?, ?)`, coordinatorID, at, at)
	mustExec(t, s, `INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, user_id, reason_code, proposal_kind, created_at)
		VALUES ('f1', ?, 'rejected', 'p-out', 'u1', 'duplicate', 'create_task', ?)`, coordinatorID, at)
	mustExec(t, s, `INSERT INTO coordinator_moveback_seen (history_row_id, coordinator_id, seen_at, state)
		VALUES (7, ?, ?, 'judged')`, coordinatorID, at)
}

func outcomesSchemaChecks(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	now := time.Now().UTC()
	seedOutcomeRows(t, s, c.ID, now)

	// An observation is unique per proposal, kind and transition.
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, user_id, reason_code, proposal_kind, created_at)
		VALUES ('f2', ?, 'rejected', 'p-out', 'u2', 'other', 'create_task', ?)`), c.ID, now); err == nil {
		t.Fatal("duplicate (proposal_id, kind, transition_key) accepted")
	}
	// A history row is judged once per coordinator.
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO coordinator_moveback_seen (history_row_id, coordinator_id, seen_at, state)
		VALUES (7, ?, ?, 'judged')`), c.ID, now); err == nil {
		t.Fatal("duplicate (history_row_id, coordinator_id) accepted")
	}
	// Another coordinator may judge the same row.
	mustExec(t, s, `INSERT INTO coordinator_moveback_seen (history_row_id, coordinator_id, seen_at, state) VALUES (7, 'other-coord', ?, 'judged')`, now)

	if _, err := NewStore(s.db, s.ro); err != nil {
		t.Fatalf("replay NewStore: %v", err)
	}
	if err := s.DeleteCoordinator(ctx, c.WorkspaceID, c.ID); err != nil {
		t.Fatalf("DeleteCoordinator: %v", err)
	}
	if n := count(t, s, "coordinator_outcomes") + count(t, s, "coordinator_feedback"); n != 0 {
		t.Fatalf("outcome and feedback rows left after coordinator delete: %d", n)
	}
	if n := count(t, s, "coordinator_moveback_seen"); n != 1 {
		t.Fatalf("seen rows = %d, want only the other coordinator's", n)
	}
}

func TestOutcomesSchema(t *testing.T) { outcomesSchemaChecks(t, newTestStore(t)) }

func TestOutcomesSchema_Postgres(t *testing.T) { outcomesSchemaChecks(t, newTestStorePostgres(t)) }

func outcomesWorkspaceDeleteChecks(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-del", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	seedOutcomeRows(t, s, c.ID, time.Now().UTC())
	if err := s.DeleteWorkspaceState(ctx, "ws-del"); err != nil {
		t.Fatalf("DeleteWorkspaceState: %v", err)
	}
	for _, table := range []string{"coordinator_outcomes", "coordinator_feedback", "coordinator_moveback_seen"} {
		if n := count(t, s, table); n != 0 {
			t.Fatalf("%s rows left after workspace delete: %d", table, n)
		}
	}
}

func TestOutcomesWorkspaceDelete(t *testing.T) { outcomesWorkspaceDeleteChecks(t, newTestStore(t)) }

func TestOutcomesWorkspaceDelete_Postgres(t *testing.T) {
	outcomesWorkspaceDeleteChecks(t, newTestStorePostgres(t))
}

func outcomesPruneChecks(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	old, fresh := now.Add(-401*24*time.Hour), now.Add(-399*24*time.Hour)
	for i, at := range []time.Time{old, old, old, fresh} {
		id := string(rune('a' + i))
		mustExec(t, s, `INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, kind, decision, decided_at, graded_at)
			VALUES (?, 'c1', 'create_task', 'approved', ?, ?)`, "p"+id, at, at)
		mustExec(t, s, `INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, user_id, reason_code, proposal_kind, created_at)
			VALUES (?, 'c1', 'rejected', ?, 'u', 'none', 'create_task', ?)`, "f"+id, "p"+id, at)
		mustExec(t, s, `INSERT INTO coordinator_moveback_seen (history_row_id, coordinator_id, seen_at, state) VALUES (?, 'c1', ?, 'judged')`, i+1, at)
	}
	// Batches of 2 force more than one pass.
	for {
		n, err := s.PruneOutcomeRows(ctx, now.Add(-outcomesRetention), 2)
		if err != nil {
			t.Fatalf("PruneOutcomeRows: %v", err)
		}
		if n == 0 {
			break
		}
	}
	for _, table := range []string{"coordinator_outcomes", "coordinator_feedback", "coordinator_moveback_seen"} {
		if n := count(t, s, table); n != 1 {
			t.Fatalf("%s rows after prune = %d, want the one fresh row", table, n)
		}
	}
}

func TestPruneOutcomeRows(t *testing.T) { outcomesPruneChecks(t, newTestStore(t)) }

func TestPruneOutcomeRows_Postgres(t *testing.T) { outcomesPruneChecks(t, newTestStorePostgres(t)) }
