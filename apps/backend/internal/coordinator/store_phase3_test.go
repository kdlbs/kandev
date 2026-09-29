package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func mustExec(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(s.db.Rebind(query), args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func insertWakeAt(t *testing.T, s *Store, c *Coordinator, id, status string, at time.Time) {
	t.Helper()
	mustExec(t, s, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'stalled', ?, ?, ?, ?)`, id, c.ID, c.WorkspaceID, "task-"+id, "ep-"+id, status, at, at)
}

func insertWake(t *testing.T, s *Store, c *Coordinator, id, status string) {
	t.Helper()
	insertWakeAt(t, s, c, id, status, time.Now().UTC())
}

// insertTurn inserts an unattended turn, open when finished is nil.
func insertTurn(t *testing.T, s *Store, c *Coordinator, id string, finished *time.Time) {
	t.Helper()
	var outcome any
	if finished != nil {
		outcome = "completed"
	}
	mustExec(t, s, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at, finished_at)
		VALUES (?, ?, 'conv', 'sess', 1, 100, ?, ?, ?)`, id, c.ID, outcome, time.Now().UTC().Add(-200*24*time.Hour), finished)
}

func insertDenial(t *testing.T, s *Store, turnID, pendingID string) {
	t.Helper()
	mustExec(t, s, `INSERT INTO coordinator_unattended_denials (turn_id, pending_id, created_at) VALUES (?, ?, ?)`, turnID, pendingID, time.Now().UTC())
}

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wakeStatus(t *testing.T, s *Store, id string) string {
	t.Helper()
	var status string
	if err := s.db.QueryRow(s.db.Rebind(`SELECT status FROM coordinator_wakes WHERE id = ?`), id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func turnExists(t *testing.T, s *Store, id string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(s.db.Rebind(`SELECT COUNT(*) FROM coordinator_unattended_turns WHERE id = ?`), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

var phase3Tables = []string{
	"coordinator_wakes", "coordinator_unattended_turns", "coordinator_unattended_denials",
	"coordinator_class_reviews", "coordinator_pending_changes", "coordinator_class_changes",
}

func seedPhase3Rows(t *testing.T, s *Store, c *Coordinator) {
	t.Helper()
	now := time.Now().UTC()
	insertWake(t, s, c, "w-"+c.ID, "pending")
	insertTurn(t, s, c, "t-"+c.ID, nil)
	insertDenial(t, s, "t-"+c.ID, "p-"+c.ID)
	mustExec(t, s, `INSERT INTO coordinator_class_reviews (id, coordinator_id, class, reviewed_by, reviewed_at, window_start, window_end, row_count) VALUES (?, ?, 'move_task', 'u', ?, ?, ?, 1)`,
		"r-"+c.ID, c.ID, now, now, now)
	mustExec(t, s, `INSERT INTO coordinator_pending_changes (id, coordinator_id, proposal_id, field, base_value, new_value, status, created_at, updated_at) VALUES (?, ?, ?, 'f', 'a', 'b', 'pending', ?, ?)`,
		"pc-"+c.ID, c.ID, "prop-"+c.ID, now, now)
	mustExec(t, s, `INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_at) VALUES (?, ?, 'move_task', 'a', 'b', ?)`,
		"cc-"+c.ID, c.ID, now)
}

func TestPhase3Schema_OpenTurnIndexIsPartial(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	fin := time.Now().UTC()
	insertTurn(t, store, c, "open-1", nil)
	insertTurn(t, store, c, "done-1", &fin)
	insertTurn(t, store, c, "done-2", &fin)
	other := newTestCoordinator(t, store, "ws-1")
	insertTurn(t, store, other, "open-other", nil)
	_, err := store.db.Exec(store.db.Rebind(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at) VALUES ('open-2', ?, 'c', 's', 1, 1, ?)`), c.ID, time.Now().UTC())
	if err == nil {
		t.Fatal("a second open turn for one coordinator must be refused")
	}
}

func TestPhase3Schema_WakeEpisodeIsUnique(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	insertWakeAt(t, store, c, "a", "pending", time.Now().UTC())
	_, err := store.db.Exec(store.db.Rebind(`INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at) VALUES ('b', ?, 'ws-1', 'task-a', 'stalled', 'ep-a', 'pending', ?, ?)`), c.ID, time.Now().UTC(), time.Now().UTC())
	if err == nil {
		t.Fatal("the same episode must not record two wakes")
	}
}

func TestPhase3Schema_ReplayKeepsRows(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	seedPhase3Rows(t, store, c)
	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("replay: %v", err)
	}
	for _, table := range phase3Tables {
		if count(t, store, table) != 1 {
			t.Fatalf("%s lost its row on replay", table)
		}
	}
}

func TestDeleteCoordinator_RemovesPhase3Rows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	a := newTestCoordinator(t, store, "ws-1")
	b := newTestCoordinator(t, store, "ws-1")
	seedPhase3Rows(t, store, a)
	seedPhase3Rows(t, store, b)
	if err := store.DeleteCoordinator(ctx, "ws-1", a.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range phase3Tables {
		if got := count(t, store, table); got != 1 {
			t.Fatalf("%s rows after delete = %d, want the other coordinator's 1", table, got)
		}
	}
	var n int
	_ = store.db.QueryRow(store.db.Rebind(`SELECT COUNT(*) FROM coordinator_unattended_denials WHERE turn_id = ?`), "t-"+a.ID).Scan(&n)
	if n != 0 {
		t.Fatal("denials of the deleted coordinator remain")
	}
}

func TestDeleteWorkspaceState_RemovesPhase3RowsOfThatWorkspaceOnly(t *testing.T) {
	store := newTestStore(t)
	a := newTestCoordinator(t, store, "ws-1")
	b := newTestCoordinator(t, store, "ws-2")
	seedPhase3Rows(t, store, a)
	seedPhase3Rows(t, store, b)
	if err := store.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}
	for _, table := range phase3Tables {
		if got := count(t, store, table); got != 1 {
			t.Fatalf("%s rows = %d, want 1", table, got)
		}
	}
	if err := store.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
}

func TestPruneWakeState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Now().UTC()
	old := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }

	insertWakeAt(t, store, c, "old-delivered", "delivered", now.Add(-31*24*time.Hour))
	insertWakeAt(t, store, c, "old-superseded", "superseded", now.Add(-31*24*time.Hour))
	insertWakeAt(t, store, c, "old-pending", "pending", now.Add(-400*24*time.Hour))
	insertWakeAt(t, store, c, "new-delivered", "delivered", now.Add(-29*24*time.Hour))
	insertTurn(t, store, c, "old-turn", old(91*24*time.Hour))
	insertDenial(t, store, "old-turn", "p1")
	insertTurn(t, store, c, "new-turn", old(89*24*time.Hour))
	insertDenial(t, store, "new-turn", "p2")
	other := newTestCoordinator(t, store, "ws-1")
	insertTurn(t, store, other, "open-turn", nil)

	turns, wakes, err := store.PruneWakeState(ctx, now)
	if err != nil || turns != 1 || wakes != 2 {
		t.Fatalf("prune = %d turns, %d wakes, %v; want 1, 2", turns, wakes, err)
	}
	for _, id := range []string{"old-pending", "new-delivered"} {
		wakeStatus(t, store, id)
	}
	if turnExists(t, store, "old-turn") || !turnExists(t, store, "new-turn") || !turnExists(t, store, "open-turn") {
		t.Fatal("wrong turns pruned")
	}
	if got := count(t, store, "coordinator_unattended_denials"); got != 1 {
		t.Fatalf("denials = %d, want 1 (the kept turn's)", got)
	}
	turns, wakes, err = store.PruneWakeState(ctx, now)
	if err != nil || turns != 0 || wakes != 0 {
		t.Fatalf("second prune = %d, %d, %v; want 0, 0", turns, wakes, err)
	}
}

func TestPruneWakeState_ServicePassthrough(t *testing.T) {
	env := newPhase3Env(t, true)
	now := time.Now().UTC()
	insertWakeAt(t, env.store, env.c, "old", "superseded", now.Add(-40*24*time.Hour))
	turns, wakes, err := env.svc.PruneWakeState(context.Background(), now)
	if err != nil || turns != 0 || wakes != 1 {
		t.Fatalf("prune = %d, %d, %v", turns, wakes, err)
	}
}

func TestWithWakeLock_MissingCoordinatorRunsNothing(t *testing.T) {
	store := newTestStore(t)
	ran := false
	err := store.WithWakeLock(context.Background(), "nope", func(coordinatorExec) error { ran = true; return nil })
	if !errors.Is(err, ErrNotFound) || ran {
		t.Fatalf("err = %v ran = %v", err, ran)
	}
}

const insertClassChange = `INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_at) VALUES ('x', ?, 'a', 'b', 'c', CURRENT_TIMESTAMP)`

func TestWithWakeLock_ErrorRollsBackAndSuccessCommits(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	boom := errors.New("boom")
	err := store.WithWakeLock(context.Background(), c.ID, func(tx coordinatorExec) error {
		if _, err := tx.ExecContext(context.Background(), insertClassChange, c.ID); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || count(t, store, "coordinator_class_changes") != 0 {
		t.Fatalf("err = %v, rows = %d", err, count(t, store, "coordinator_class_changes"))
	}
	err = store.WithWakeLock(context.Background(), c.ID, func(tx coordinatorExec) error {
		_, err := tx.ExecContext(context.Background(), insertClassChange, c.ID)
		return err
	})
	if err != nil || count(t, store, "coordinator_class_changes") != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestWithWakeLock_CancelledContextRollsBack(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx, cancel := context.WithCancel(context.Background())
	err := store.WithWakeLock(ctx, c.ID, func(tx coordinatorExec) error {
		if _, err := tx.ExecContext(context.Background(), insertClassChange, c.ID); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	})
	if err == nil || count(t, store, "coordinator_class_changes") != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestWithWakeLock_SerialisesCallersForOneCoordinator(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	assertSerialised(t, store, c.ID)
}
