package ledger

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func (f *fixture) unattended(id string, sessionTurnID, reserved, outcome *string, started time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, session_turn_id, reserved_turn_id, wake_count, start_ceiling_subcents, outcome, started_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, 100, ?, ?)`, id, f.coord.ID, testConvTask, testSession, sessionTurnID, reserved, outcome, started)
}

func (f *fixture) wake(id, kind, unattendedID string) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, turn_id, created_at, updated_at)
		VALUES (?, ?, 'ws-1', 'task-x', ?, ?, 'delivered', ?, ?, ?)`, id, f.coord.ID, kind, id, unattendedID, f.now, f.now)
}

func sp(s string) *string { return &s }

func (f *fixture) unattendedLink(id string) *string {
	f.t.Helper()
	var link *string
	if err := f.db.QueryRow(f.db.Rebind(`SELECT ledger_turn_id FROM coordinator_unattended_turns WHERE id = ?`), id).Scan(&link); err != nil {
		f.t.Fatal(err)
	}
	return link
}

func TestRecorder_WakeTriggerCopiesKindsAndLinks(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.unattended("u1", nil, sp("st-1"), nil, s)
	f.wake("w-b", "stall", "u1")
	f.wake("w-a", "question", "u1")
	f.start("st-1", s)

	row := f.oneTurn()
	if row.Trigger != TriggerWake || row.WakeKinds != `["question","stall"]` {
		t.Fatalf("row = %+v", row)
	}
	if link := f.unattendedLink("u1"); link == nil || *link != row.ID {
		t.Fatalf("unattended link = %v, want %s", link, row.ID)
	}
}

func TestRecorder_WakeMatchExclusions(t *testing.T) {
	cases := map[string]struct {
		session, reserved, outcome *string
	}{
		"send_failed":                 {nil, sp("st-1"), sp("send_failed")},
		"settled while unbound":       {nil, sp("st-1"), sp("stopped_by_pause")},
		"unbound without reservation": {nil, nil, nil},
		"empty reservation":           {nil, sp(""), nil},
		"other turn":                  {sp("st-other"), sp("st-other"), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			s := f.at(0)
			f.unattended("u1", tc.session, tc.reserved, tc.outcome, s)
			f.wake("w1", "question", "u1")
			f.start("st-1", s)
			row := f.oneTurn()
			if row.Trigger != TriggerMessage || row.WakeKinds != "[]" {
				t.Fatalf("manager message mislabelled: %+v", row)
			}
			if link := f.unattendedLink("u1"); link != nil {
				t.Fatalf("excluded row linked to %s", *link)
			}
		})
	}
}

func TestRecorder_NewestMatchingUnattendedRowWins(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.unattended("u-old", sp("st-1"), nil, sp("completed"), s.Add(-time.Hour))
	f.unattended("u-new", sp("st-1"), nil, nil, s)
	f.wake("w1", "permission", "u-new")
	f.start("st-1", s)
	if link := f.unattendedLink("u-new"); link == nil {
		t.Fatal("newest row not linked")
	}
	if link := f.unattendedLink("u-old"); link != nil {
		t.Fatal("older row linked")
	}
}

func TestRecorder_StartBeforeReservationIsCorrectedAtCompletion(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.start("st-1", s)
	if got := f.oneTurn().Trigger; got != TriggerMessage {
		t.Fatalf("trigger at start = %s", got)
	}
	f.unattended("u1", sp("st-1"), sp("st-1"), sp("completed"), s)
	f.wake("w1", "question", "u1")
	f.complete("st-1", s, s.Add(time.Second))

	row := f.oneTurn()
	if row.Trigger != TriggerWake || row.WakeKinds != `["question"]` || *row.Outcome != "completed" {
		t.Fatalf("row = %+v", row)
	}
	if link := f.unattendedLink("u1"); link == nil || *link != row.ID {
		t.Fatalf("link = %v", link)
	}
}

func TestRecorder_NothingElseChangesAStoredTrigger(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.unattended("u1", sp("st-1"), nil, sp("completed"), s)
	f.wake("w1", "stall", "u1")
	f.start("st-1", s)
	f.exec(`UPDATE coordinator_turns SET "trigger" = 'dream'`)
	f.complete("st-1", s, s.Add(time.Second))
	if got := f.oneTurn().Trigger; got != "dream" {
		t.Fatalf("completion rewrote trigger to %s", got)
	}
}

func TestRecorder_OutcomeFromBoundUnattendedRow(t *testing.T) {
	for _, outcome := range []string{"stopped_at_ceiling", "stopped_by_pause", "interrupted", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			f := newFixture(t)
			s := f.at(0)
			f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
			f.unattended("u1", sp("st-1"), nil, sp(outcome), s)
			f.start("st-1", s)
			f.complete("st-1", s, s.Add(time.Second))
			row := f.oneTurn()
			if *row.Outcome != outcome || *row.Verdict != VerdictBlocked {
				t.Fatalf("row = %+v", row)
			}
		})
	}
}

func TestRecorder_UnsettledBoundRowIsReadAgainThenFallsBack(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.unattended("u1", sp("st-1"), nil, nil, s)
	f.start("st-1", s)
	f.complete("st-1", s, s.Add(time.Second))
	if f.oneTurn().FinishedAt != nil {
		t.Fatal("completion must wait for the unattended settle")
	}
	f.exec(`UPDATE coordinator_unattended_turns SET outcome = 'stopped_at_ceiling' WHERE id = 'u1'`)
	waitFor(t, func() bool { return f.oneTurn().FinishedAt != nil })
	if got := *f.oneTurn().Outcome; got != "stopped_at_ceiling" {
		t.Fatalf("outcome = %s", got)
	}
}

func TestRecorder_UnsettledBoundRowFallsBackToSessionStateAfterRetries(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'CANCELLED')`, testSession)
	f.unattended("u1", sp("st-1"), nil, nil, s)
	f.start("st-1", s)
	f.complete("st-1", s, s.Add(time.Second))
	waitFor(t, func() bool { return f.oneTurn().FinishedAt != nil })
	if got := *f.oneTurn().Outcome; got != "cancelled" {
		t.Fatalf("outcome = %s", got)
	}
}

func TestRecorder_RowCreatedAtCompletionTakesTheWakeTrigger(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_session_turns (id, started_at) VALUES ('st-1', ?)`, s)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.unattended("u1", sp("st-1"), sp("st-1"), sp("completed"), s)
	f.wake("w1", "question", "u1")
	f.complete("st-1", s, s.Add(time.Second)) // the start event was lost

	row := f.oneTurn()
	if row.Trigger != TriggerWake || row.WakeKinds != `["question"]` || row.SnapshotHash != "" {
		t.Fatalf("row = %+v", row)
	}
	if link := f.unattendedLink("u1"); link == nil || *link != row.ID {
		t.Fatalf("link = %v, want %s", link, row.ID)
	}
}

func TestRecorder_RowCreatedAtCompletionExcludesUnusableWakeRows(t *testing.T) {
	cases := map[string]*string{"send_failed": sp("send_failed"), "settled while unbound": sp("stopped_by_pause")}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			s := f.at(0)
			f.exec(`INSERT INTO task_session_turns (id, started_at) VALUES ('st-1', ?)`, s)
			f.unattended("u1", nil, sp("st-1"), outcome, s)
			f.wake("w1", "question", "u1")
			f.complete("st-1", s, s.Add(time.Second))
			row := f.oneTurn()
			if row.Trigger != TriggerMessage || row.WakeKinds != "[]" {
				t.Fatalf("row = %+v", row)
			}
			if link := f.unattendedLink("u1"); link != nil {
				t.Fatalf("excluded row linked to %s", *link)
			}
		})
	}
}

func TestRecorder_WakeKindsAreCappedAtTwenty(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.unattended("u1", nil, sp("st-1"), nil, s)
	for i := 0; i < 21; i++ {
		f.wake(fmt.Sprintf("w%02d", i), fmt.Sprintf("kind%02d", i), "u1")
	}
	f.start("st-1", s)
	var kinds []string
	if err := json.Unmarshal([]byte(f.oneTurn().WakeKinds), &kinds); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 20 {
		t.Fatalf("wake kinds = %d, want 20", len(kinds))
	}
}
