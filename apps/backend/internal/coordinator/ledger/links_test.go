package ledger

import (
	"testing"
	"time"
)

func (f *fixture) proposal(id, coordinatorID string, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at, kind)
		VALUES (?, ?, 'ws-1', 'pending', '{}', ?, ?, 'create_task')`, id, coordinatorID, at, at)
}

func (f *fixture) activity(id, outcome, auth string, undoOf *string, unattendedTurn *string, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_activity (id, coordinator_id, workspace_id, action_class, outcome, "authorization", undo_of_id, unattended_turn_id, created_at, updated_at)
		VALUES (?, ?, 'ws-1', 'create_task', ?, ?, ?, ?, ?, ?)`, id, f.coord.ID, outcome, auth, undoOf, unattendedTurn, at, at)
}

func (f *fixture) turnIDOf(table, id string) *string {
	f.t.Helper()
	var v *string
	if err := f.db.QueryRow(f.db.Rebind(`SELECT turn_id FROM `+table+` WHERE id = ?`), id).Scan(&v); err != nil {
		f.t.Fatal(err)
	}
	return v
}

func TestRepair_FillsNullLinksInsideTheWindowOnly(t *testing.T) {
	f := newFixture(t)
	s, e := f.at(0), f.at(10*time.Second)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.start("st-1", s)

	f.proposal("p-start", f.coord.ID, s) // inclusive lower bound
	f.proposal("p-end", f.coord.ID, e)   // inclusive upper bound
	f.proposal("p-before", f.coord.ID, s.Add(-time.Millisecond))
	f.proposal("p-after", f.coord.ID, e.Add(time.Millisecond))
	f.activity("a-proposed", "proposed", "requires_approval", nil, nil, f.at(time.Second))
	f.activity("a-denied", "refused", "denied", nil, nil, f.at(time.Second))
	f.activity("a-manager", "approved", "requires_approval", nil, nil, f.at(time.Second))
	f.activity("a-undo", "undone", "denied", sp("x"), nil, f.at(time.Second))
	f.complete("st-1", s, e)

	turn := f.oneTurn().ID
	for id, want := range map[string]bool{"p-start": true, "p-end": true, "p-before": false, "p-after": false} {
		if got := f.turnIDOf("coordinator_proposals", id); want != (got != nil && *got == turn) {
			t.Fatalf("proposal %s turn_id = %v, linked want %v", id, got, want)
		}
	}
	for id, want := range map[string]bool{"a-proposed": true, "a-denied": true, "a-manager": false, "a-undo": false} {
		if got := f.turnIDOf("coordinator_activity", id); want != (got != nil && *got == turn) {
			t.Fatalf("activity %s turn_id = %v, linked want %v", id, got, want)
		}
	}
	if v := *f.oneTurn().Verdict; v != VerdictProposed {
		t.Fatalf("verdict = %s, want proposed from repaired links", v)
	}
}

func TestRepair_OverlappingTurnLeavesNullNeverGuesses(t *testing.T) {
	f := newFixture(t)
	s, e := f.at(0), f.at(10*time.Second)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.start("st-1", s)
	// A concurrent unfinished turn of the same coordinator (another session).
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at) VALUES ('dream', ?, 'sess-dream', 'st-d', 'dream', ?)`, f.coord.ID, f.at(5*time.Second))
	f.proposal("p1", f.coord.ID, f.at(time.Second))
	f.complete("st-1", s, e)
	if got := f.turnIDOf("coordinator_proposals", "p1"); got != nil {
		t.Fatalf("overlap must leave NULL, got %v", *got)
	}
}

func TestRepair_SharedBoundaryInstantCountsAsOverlap(t *testing.T) {
	f := newFixture(t)
	s, e := f.at(0), f.at(10*time.Second)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at) VALUES ('prev', ?, 'sess-p', 'st-p', 'wake', ?, ?)`, f.coord.ID, f.at(-time.Minute), s)
	f.start("st-1", s)
	f.proposal("p1", f.coord.ID, f.at(time.Second))
	f.complete("st-1", s, e)
	if got := f.turnIDOf("coordinator_proposals", "p1"); got != nil {
		t.Fatalf("a turn ending at this turn's start overlaps; got %v", *got)
	}
}

func TestRepair_OtherCoordinatorsRowsAreUntouched(t *testing.T) {
	f := newFixture(t)
	s := f.at(0)
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
	f.start("st-1", s)
	f.proposal("foreign", "other-coordinator", f.at(time.Second))
	f.complete("st-1", s, f.at(10*time.Second))
	if got := f.turnIDOf("coordinator_proposals", "foreign"); got != nil {
		t.Fatalf("foreign proposal linked to %s", *got)
	}
}

func TestVerdict_ActedFromBoundAutomaticApprovalsOnly(t *testing.T) {
	cases := map[string]struct {
		outcome, auth string
		want          string
	}{
		"approved automatic": {"approved", "automatic", VerdictActed},
		"failed automatic":   {"failed", "automatic", VerdictNothingNeeded},
		"manager approval":   {"approved", "requires_approval", VerdictNothingNeeded},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			s := f.at(0)
			f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'IDLE')`, testSession)
			f.unattended("u1", sp("st-1"), nil, sp("completed"), s)
			f.activity("a1", tc.outcome, tc.auth, nil, sp("u1"), f.at(time.Second))
			f.activity("a-other", "approved", "automatic", nil, sp("u-unbound"), f.at(time.Second))
			f.start("st-1", s)
			f.complete("st-1", s, f.at(10*time.Second))
			if v := *f.oneTurn().Verdict; v != tc.want {
				t.Fatalf("verdict = %s, want %s", v, tc.want)
			}
		})
	}
}

func TestVerdict_NeedsYouReadsPendingInteractionAtGradeTime(t *testing.T) {
	cases := map[string]struct {
		pending bool
		err     error
		want    string
	}{
		"still pending": {true, nil, VerdictNeedsYou},
		"answered":      {false, nil, VerdictNothingNeeded},
		"read failed":   {true, errTest, VerdictNothingNeeded},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.pending, f.pendingErr = tc.pending, tc.err
			s := f.at(0)
			f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'WAITING_FOR_INPUT')`, testSession)
			f.unattended("u1", sp("st-1"), nil, sp("completed"), s)
			f.wake("w1", "question", "u1")
			f.start("st-1", s)
			f.complete("st-1", s, f.at(time.Second))
			if v := *f.oneTurn().Verdict; v != tc.want {
				t.Fatalf("verdict = %s, want %s", v, tc.want)
			}
		})
	}
}

func TestVerdict_ManagerMessageIsNeverNeedsYou(t *testing.T) {
	f := newFixture(t)
	f.pending = true
	f.exec(`INSERT INTO task_sessions (id, state) VALUES (?, 'WAITING_FOR_INPUT')`, testSession)
	f.unattended("u1", nil, nil, nil, f.at(0)) // an open delivery that does not match this turn
	f.wake("w1", "question", "u1")
	s := f.at(time.Second)
	f.start("st-1", s)
	f.complete("st-1", s, f.at(2*time.Second))
	if v := *f.oneTurn().Verdict; v != VerdictNothingNeeded {
		t.Fatalf("verdict = %s", v)
	}
}
