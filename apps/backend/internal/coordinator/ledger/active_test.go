package ledger

import (
	"testing"
	"time"
)

func TestActive_RedeliveredStartKeepsTheEntry(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.start("st-1", s)
	id := f.l.ActiveTurnID(testSession)
	f.start("st-1", s)
	if got := f.l.ActiveTurnID(testSession); got != id || id == "" {
		t.Fatalf("entry changed: %q -> %q", id, got)
	}
}

func TestActive_StartOfAnEarlierTurnNeverReplacesALaterEntry(t *testing.T) {
	f := newFixture(t)
	f.start("st-2", f.at(time.Minute))
	later := f.l.ActiveTurnID(testSession)
	f.start("st-1", f.at(0))
	if got := f.l.ActiveTurnID(testSession); got != later {
		t.Fatalf("earlier turn replaced the later entry: %q", got)
	}
}

func TestActive_CompletionOfAnEarlierTurnLeavesALaterEntry(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	s1 := f.at(0)
	f.start("st-1", s1)
	f.start("st-2", f.at(time.Minute))
	later := f.l.ActiveTurnID(testSession)
	f.complete("st-1", s1, f.at(time.Second))
	if got := f.l.ActiveTurnID(testSession); got != later {
		t.Fatalf("completing st-1 removed the entry of st-2: %q", got)
	}
}

func TestActive_FailedInsertRemovesItsOwnEntryAndCounts(t *testing.T) {
	f := newFixture(t)
	f.exec(`DROP INDEX idx_coordinator_turns_session_turn`)
	f.exec(`ALTER TABLE coordinator_turns RENAME TO coordinator_turns_gone`)
	before := FailureCount(StageStart)
	f.start("st-1", f.at(0))
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("dangling entry %q after a failed insert", got)
	}
	if FailureCount(StageStart) != before+1 {
		t.Fatal("failure not counted under stage start")
	}
}

func TestActive_ExistingFinishedRowRemovesTheGeneratedEntry(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at, outcome, verdict)
		VALUES ('done', ?, ?, 'st-1', 'message', ?, ?, 'completed', 'nothing_needed')`, f.coord.ID, testSession, s, s.Add(time.Second))
	f.start("st-1", s)
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("entry %q points at a row that is not this turn's open row", got)
	}
	if n := len(f.turns()); n != 1 {
		t.Fatalf("rows = %d", n)
	}
}

func TestActive_ExistingUnfinishedRowReplacesTheGeneratedEntry(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at)
		VALUES ('open', ?, ?, 'st-1', 'message', ?)`, f.coord.ID, testSession, s)
	f.start("st-1", s)
	if got := f.l.ActiveTurnID(testSession); got != "open" {
		t.Fatalf("entry = %q, want the existing row", got)
	}
}

func TestActive_RebuildAtStartTakesNewestUnfinishedRowPerSession(t *testing.T) {
	f := newFixture(t)
	for _, r := range []struct {
		id, session, turn string
		at                time.Duration
		finished          bool
	}{
		{"old", "sA", "t1", 0, false},
		{"new", "sA", "t2", time.Minute, false},
		{"done", "sA", "t3", 2 * time.Minute, true},
		{"other", "sB", "t4", 0, false},
	} {
		f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at) VALUES (?, ?, ?, ?, 'message', ?)`,
			r.id, f.coord.ID, r.session, r.turn, f.at(r.at))
		if r.finished {
			f.exec(`UPDATE coordinator_turns SET finished_at = ? WHERE id = ?`, f.at(r.at+time.Second), r.id)
		}
	}
	f.l.Start(t.Context())
	if got := f.l.ActiveTurnID("sA"); got != "new" {
		t.Fatalf("sA = %q, want new", got)
	}
	if got := f.l.ActiveTurnID("sB"); got != "other" {
		t.Fatalf("sB = %q", got)
	}
}

func TestActive_ActiveTurnIDNeverTouchesTheDatabase(t *testing.T) {
	f := newFixture(t)
	f.start("st-1", f.at(0))
	f.exec(`ALTER TABLE coordinator_turns RENAME TO coordinator_turns_gone`)
	if f.l.ActiveTurnID(testSession) == "" {
		t.Fatal("entry lost")
	}
}
