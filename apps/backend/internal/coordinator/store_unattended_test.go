package coordinator

import (
	"context"
	"testing"
	"time"
)

func insertBoundTurn(t *testing.T, s *Store, c *Coordinator, id, task, session string, sessionTurn any, outcome any) {
	t.Helper()
	mustExec(t, s, `INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, outcome, started_at)
		VALUES (?, ?, ?, ?, ?, 1, 100, ?, ?)`, id, c.ID, task, session, sessionTurn, outcome, time.Now().UTC())
}

func deniedCount(t *testing.T, s *Store, turnID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(s.db.Rebind(`SELECT denied_permissions FROM coordinator_unattended_turns WHERE id = ?`), turnID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func denialRows(t *testing.T, s *Store, turnID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(s.db.Rebind(`SELECT COUNT(*) FROM coordinator_unattended_denials WHERE turn_id = ?`), turnID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRecordUnattendedDenial_CountsOncePerPendingID(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		id, matched, err := s.RecordUnattendedDenial(ctx, "conv", "sess", "pending-1", "st-1")
		if err != nil || !matched || id != "turn-1" {
			t.Fatalf("attempt %d: id=%q matched=%v err=%v", i, id, matched, err)
		}
	}
	if got := deniedCount(t, s, "turn-1"); got != 1 {
		t.Fatalf("denied_permissions = %d, want 1 for one pending id", got)
	}
	if _, matched, _ := s.RecordUnattendedDenial(ctx, "conv", "sess", "pending-2", "st-1"); !matched {
		t.Fatal("a second pending id must match")
	}
	if got := deniedCount(t, s, "turn-1"); got != 2 {
		t.Fatalf("denied_permissions = %d, want 2", got)
	}
}

func TestRecordUnattendedDenial_UnboundTurnMatchesBySession(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", nil, nil)

	id, matched, err := s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-1", "st-any")
	if err != nil || !matched || id != "turn-1" {
		t.Fatalf("id=%q matched=%v err=%v", id, matched, err)
	}
	if deniedCount(t, s, "turn-1") != 1 {
		t.Fatal("an unbound window request must be counted")
	}
}

func TestRecordUnattendedDenial_NoMatch(t *testing.T) {
	ctx := context.Background()
	fin := "completed"
	cases := []struct {
		name                       string
		task, session, sessionTurn string
		bound                      any
		outcome                    any
	}{
		{"later session turn", "conv", "sess", "st-2", "st-1", nil},
		{"other session", "conv", "sess-2", "st-1", "st-1", nil},
		{"other task", "conv-2", "sess", "st-1", "st-1", nil},
		{"settled turn", "conv", "sess", "st-1", "st-1", fin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			c := newTestCoordinator(t, s, "ws-1")
			insertBoundTurn(t, s, c, "turn-1", "conv", "sess", tc.bound, tc.outcome)
			id, matched, err := s.RecordUnattendedDenial(ctx, tc.task, tc.session, "pending-1", tc.sessionTurn)
			if err != nil || matched || id != "" {
				t.Fatalf("id=%q matched=%v err=%v, want no match", id, matched, err)
			}
			if denialRows(t, s, "turn-1") != 0 || deniedCount(t, s, "turn-1") != 0 {
				t.Fatal("a non-matching request must record and count nothing")
			}
		})
	}
}

func TestRecordUnattendedDenial_SettledBetweenLookupAndInsertRecordsNothing(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)
	s.afterUnattendedLookup = func() {
		mustExec(t, s, `UPDATE coordinator_unattended_turns SET outcome = 'completed' WHERE id = 'turn-1'`)
	}
	_, matched, err := s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-1", "st-1")
	if err != nil || matched {
		t.Fatalf("matched=%v err=%v, want no match after settle", matched, err)
	}
	if denialRows(t, s, "turn-1") != 0 || deniedCount(t, s, "turn-1") != 0 {
		t.Fatal("a turn settled before the insert must record and count nothing")
	}
}

func TestListOpenDenials_ScopedToCoordinatorAndOpenTurns(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	other := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "open", "conv", "sess", "st-1", nil)
	insertBoundTurn(t, s, other, "other-open", "conv-o", "sess-o", "st-o", nil)
	insertBoundTurn(t, s, c, "settled", "conv", "sess-old", "st-0", "completed")
	insertDenial(t, s, "open", "p-1")
	insertDenial(t, s, "other-open", "p-2")
	insertDenial(t, s, "settled", "p-3")

	got, err := s.ListOpenDenials(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PendingID != "p-1" || got[0].TurnID != "open" || got[0].TaskID != "conv" || got[0].SessionID != "sess" {
		t.Fatalf("open denials = %+v", got)
	}
	empty, err := s.ListOpenDenials(context.Background(), "no-such")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty selection = %+v err=%v", empty, err)
	}
}
