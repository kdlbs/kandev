package ledger

import (
	"fmt"
	"testing"
	"time"
)

type callRow struct {
	ID      int64   `db:"id"`
	TurnID  string  `db:"turn_id"`
	Action  string  `db:"action"`
	Target  *string `db:"target_task_id"`
	Allowed bool    `db:"allowed"`
}

func (f *fixture) callRows() []callRow {
	f.t.Helper()
	var rows []callRow
	if err := f.db.Select(&rows, `SELECT id, turn_id, action, target_task_id, allowed FROM coordinator_turn_calls ORDER BY id`); err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *fixture) startWriter() {
	f.l.Start(f.t.Context())
}

func TestCalls_RecordedInOrderWithoutArguments(t *testing.T) {
	f := newFixture(t)
	f.startWriter()
	f.start("st-1", f.at(0))
	f.l.Call(testSession, "move_task_kandev", "task-1", true)
	f.l.Call(testSession, "message_task_kandev", "", false)
	f.l.drainCalls(t.Context())

	rows := f.callRows()
	if len(rows) != 2 || rows[0].Action != "move_task_kandev" || !rows[0].Allowed || rows[1].Allowed {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Target == nil || *rows[0].Target != "task-1" || rows[1].Target != nil {
		t.Fatalf("targets = %v / %v", rows[0].Target, rows[1].Target)
	}
	if rows[0].ID >= rows[1].ID || rows[0].TurnID != f.oneTurn().ID {
		t.Fatalf("order or turn wrong: %+v", rows)
	}
}

func TestCalls_HundredCapDropsTheHundredAndFirst(t *testing.T) {
	f := newFixture(t)
	f.startWriter()
	f.start("st-1", f.at(0))
	for i := 0; i < 101; i++ {
		f.l.Call(testSession, fmt.Sprintf("a%d", i), "", true)
	}
	f.l.drainCalls(t.Context())
	if n := len(f.callRows()); n != 100 {
		t.Fatalf("call rows = %d, want 100", n)
	}
	if !f.oneTurn().Truncated {
		t.Fatal("calls_truncated not set")
	}
}

func TestCalls_FullQueueDropsMarksTurnAndCounts(t *testing.T) {
	f := newFixture(t) // writer deliberately not started, so the queue fills
	f.start("st-1", f.at(0))
	before := FailureCount(StageCallQueueFull)
	for i := 0; i < callQueueSize+1; i++ {
		f.l.Call(testSession, "a", "", true)
	}
	if got := FailureCount(StageCallQueueFull) - before; got != 1 {
		t.Fatalf("queue-full count = %d, want 1", got)
	}
	f.l.applyTruncations()
	if !f.oneTurn().Truncated {
		t.Fatal("full queue did not set calls_truncated on the active turn")
	}
}

func TestCalls_CallBeforeStartIsParkedThenAttributed(t *testing.T) {
	f := newFixture(t)
	f.l.retryEvery = 20 * time.Millisecond
	f.startWriter()
	f.l.Call(testSession, "late_start", "", true)
	if !f.l.calls.retryPending(testSession) {
		waitFor(t, func() bool { return f.l.calls.retryPending(testSession) })
	}
	f.start("st-1", f.at(-time.Hour)) // the turn began before the call was enqueued
	waitFor(t, func() bool { return len(f.callRows()) == 1 })
	waitFor(t, func() bool { return !f.l.calls.retryPending(testSession) })
}

func TestCalls_UnattributedCallIsDroppedAfterRetries(t *testing.T) {
	f := newFixture(t)
	f.startWriter()
	before := FailureCount(StageCallUnattributed)
	f.l.Call(testSession, "orphan", "", true)
	waitFor(t, func() bool { return FailureCount(StageCallUnattributed) == before+1 })
	if f.l.calls.retryPending(testSession) {
		t.Fatal("dropped call left on the retry list")
	}
	if n := len(f.callRows()); n != 0 {
		t.Fatalf("rows = %d", n)
	}
}

func TestCalls_LateWriteIsNeverChargedToTheNextTurn(t *testing.T) {
	f := newFixture(t)
	t1, t2 := f.at(0), f.at(10*time.Second)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.start("st-1", t1)
	f.complete("st-1", t1, t1.Add(2*time.Second))
	f.start("st-2", t2)

	f.setClock(t1.Add(time.Second))
	f.startWriter()
	f.l.Call(testSession, "belongs_to_first", "", true)
	f.l.drainCalls(t.Context())

	rows := f.callRows()
	var first string
	for _, r := range f.turns() {
		if r.FinishedAt != nil {
			first = r.ID
		}
	}
	if len(rows) != 1 || rows[0].TurnID != first {
		t.Fatalf("rows = %+v, first turn %s", rows, first)
	}
}

func TestCalls_CompletionDrainsQueueIntoVerdict(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.startWriter()
	s := f.at(0)
	f.start("st-1", s)
	f.l.Call(testSession, "move_task_kandev", "t", false)
	f.l.Call(testSession, "message_task_kandev", "t", false)
	f.complete("st-1", s, s.Add(time.Second))
	if v := *f.oneTurn().Verdict; v != VerdictBlocked {
		t.Fatalf("verdict = %s, want blocked (every call refused)", v)
	}
}

func TestCalls_TruncatedDigestNeverBlocksByRefusals(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.startWriter()
	s := f.at(0)
	f.start("st-1", s)
	f.l.Call(testSession, "a", "", false)
	f.l.drainCalls(t.Context())
	f.exec(`UPDATE coordinator_turns SET calls_truncated = ?`, true)
	f.complete("st-1", s, s.Add(time.Second))
	if v := *f.oneTurn().Verdict; v != VerdictNothingNeeded {
		t.Fatalf("verdict = %s", v)
	}
}

func TestCalls_ParkedEntryOnRetryListPreventsBlocked(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.l.retryEvery = time.Hour // stays parked
	f.startWriter()
	s := f.at(0)
	f.start("st-1", s)
	f.l.Call(testSession, "settled_refusal", "", false)
	f.l.drainCalls(t.Context())
	f.setClock(s.Add(-time.Hour)) // resolves to no turn: parked
	f.l.Call(testSession, "unresolvable", "", false)
	f.l.drainCalls(t.Context())
	f.resetClock()
	f.complete("st-1", s, s.Add(time.Second))
	if v := *f.oneTurn().Verdict; v != VerdictNothingNeeded {
		t.Fatalf("verdict = %s, want nothing_needed while a call is parked", v)
	}
}

func TestCalls_NoCallsIsNotBlocked(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	s := f.at(0)
	f.start("st-1", s)
	f.complete("st-1", s, s.Add(time.Second))
	if v := *f.oneTurn().Verdict; v != VerdictNothingNeeded {
		t.Fatalf("verdict = %s", v)
	}
}
