package recorder

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

type fakeAgreement struct{ rated, agreeing int64 }

func (a fakeAgreement) Agreement(context.Context, string, time.Time, time.Time) (int64, int64, error) {
	return a.rated, a.agreeing, nil
}

func (f *fixture) measures(a AgreementSource) *Measures { return NewMeasures(f.db, a) }

func (f *fixture) read(days int, a AgreementSource) *coordinator.OutcomeMeasures {
	f.t.Helper()
	out, err := f.measures(a).Measures(context.Background(), f.coord.ID, days, f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) seedOutcome(id, decision string, automatic bool, edited string, decidedAt time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, kind, decision, automatic, decided_at, edited_fields, graded_at)
		VALUES (?, ?, 'create_task', ?, ?, ?, ?, ?)`, id, f.coord.ID, decision, automatic, decidedAt, edited, decidedAt)
}

func want(t *testing.T, name string, m coordinator.OutcomeMeasure, value float64, num, den int64) {
	t.Helper()
	if m.Value == nil || *m.Value != value || m.Numerator != num || m.Denominator != den || m.NullReason != "" {
		t.Fatalf("%s = %+v (value %v), want %v %d/%d", name, m, m.Value, value, num, den)
	}
}

func wantNull(t *testing.T, name string, m coordinator.OutcomeMeasure, reason string) {
	t.Helper()
	if m.Value != nil || m.NullReason != reason {
		t.Fatalf("%s = %+v, want null %s", name, m, reason)
	}
}

func TestMeasures_EmptyWindowIsNullNeverZero(t *testing.T) {
	f := newFixture(t)
	out := f.read(30, nil)
	wantNull(t, "approval", out.ApprovalWithoutEdit, coordinator.NullNoData)
	wantNull(t, "recurrence", out.OverrideRecurrence, coordinator.NullNoData)
	wantNull(t, "dollars", out.DollarsPerMerged, coordinator.NullNoData)
	wantNull(t, "median", out.MedianWaitSeconds, coordinator.NullNoData)
	wantNull(t, "agreement", out.Agreement, coordinator.NullNoData)
}

func TestMeasures_ApprovalWithoutEdit(t *testing.T) {
	f := newFixture(t)
	d := f.at(-time.Hour)
	f.seedOutcome("a", "approved", false, "[]", d)
	f.seedOutcome("b", "edited", false, `["title"]`, d)
	f.seedOutcome("c", "rejected", false, "[]", d)
	f.seedOutcome("d", "undone", false, "[]", d)
	f.seedOutcome("e", "undone", false, `["title"]`, d)
	f.seedOutcome("auto", "approved", true, "[]", d)
	f.seedOutcome("ret", "returned", false, "[]", d)
	f.seedOutcome("old", "approved", false, "[]", f.at(-31*24*time.Hour))
	want(t, "approval", f.read(30, nil).ApprovalWithoutEdit, 0.4, 2, 5)
}

func (f *fixture) seedFeedback(id, kind, reason string, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, user_id, reason_code, proposal_kind, to_step_id, transition_key, created_at)
		VALUES (?, ?, ?, ?, 'u', ?, 'create_task', '', ?, ?)`, id, f.coord.ID, kind, "p-"+id, reason, id, at)
}

func TestMeasures_OverrideRecurrence(t *testing.T) {
	f := newFixture(t)
	day := 24 * time.Hour
	f.seedFeedback("a", "rejected", "duplicate", f.at(-40*day)) // before the window, inside the look-back
	f.seedFeedback("b", "rejected", "duplicate", f.at(-10*day)) // recurs through a
	f.seedFeedback("c", "rejected", "duplicate", f.at(-9*day))  // recurs through b
	f.seedFeedback("d", "rejected", "other", f.at(-5*day))      // new key
	want(t, "recurrence", f.read(30, nil).OverrideRecurrence, 2.0/3.0, 2, 3)
	// A prior row more than 30 days earlier does not count.
	g := newFixture(t)
	g.seedFeedback("a", "undone", "none", g.at(-80*day))
	g.seedFeedback("b", "undone", "none", g.at(-45*day))
	want(t, "gap", g.read(90, nil).OverrideRecurrence, 0, 0, 2)
}

func TestMeasures_DollarsPerMergedTask(t *testing.T) {
	f := newFixture(t)
	f.seedOutcome("p1", "approved", false, "[]", f.at(-time.Hour))
	f.exec(`UPDATE coordinator_outcomes SET task_id = 't1', merged_at = ? WHERE proposal_id = 'p1'`, f.at(-time.Hour))
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at) VALUES ('turn1', ?, 's', 'st1', 'wake', ?)`, f.coord.ID, f.at(-2*time.Hour))
	wantNull(t, "no usage", f.read(30, nil).DollarsPerMerged, coordinator.NullCostUnknown)
	f.exec(`INSERT INTO task_usage_events (task_id, session_id, turn_id, cost_subcents) VALUES ('x', 's', 'st1', 25000)`)
	want(t, "priced", f.read(30, nil).DollarsPerMerged, 2.5, 25000, 1)
	f.exec(`INSERT INTO task_usage_events (task_id, session_id, turn_id, cost_subcents, cost_source) VALUES ('x', 's', 'st1', 1, 'unpriced')`)
	wantNull(t, "unpriced", f.read(30, nil).DollarsPerMerged, coordinator.NullCostUnknown)
}

func (f *fixture) seedWait(id string, wait time.Duration, decidedAt time.Time) {
	f.t.Helper()
	f.proposal(id, "approved", "create_task", "", decidedAt)
	f.exec(`UPDATE coordinator_proposals SET created_at = ? WHERE id = ?`, decidedAt.Add(-wait), id)
	f.seedOutcome(id, "approved", false, "[]", decidedAt)
}

func TestMeasures_MedianWaitOddEvenAndCap(t *testing.T) {
	f := newFixture(t)
	for i, w := range []time.Duration{10 * time.Second, 30 * time.Second, 20 * time.Second} {
		f.seedWait("w"+string(rune('a'+i)), w, f.at(-time.Hour))
	}
	m := f.read(30, nil).MedianWaitSeconds
	want(t, "odd", m, 20, 3, 3)
	f.seedWait("wd", 41*time.Second, f.at(-time.Hour))
	if m := f.read(30, nil).MedianWaitSeconds; m.Value == nil || *m.Value != 25 {
		t.Fatalf("even = %+v", m)
	}
	f.seedWait("we", 1500*time.Millisecond+10*time.Second, f.at(-time.Hour))
	f.seedWait("wf", 12500*time.Millisecond, f.at(-time.Hour))
	orig := medianCap
	medianCap = 2
	defer func() { medianCap = orig }()
	capped := f.read(30, nil).MedianWaitSeconds
	if !capped.Capped || capped.Numerator != 2 || capped.Denominator != 6 {
		t.Fatalf("capped = %+v", capped)
	}
}

func TestMeasures_MedianEvenRoundsDownToSecond(t *testing.T) {
	f := newFixture(t)
	f.seedWait("a", 10*time.Second, f.at(-time.Hour))
	f.seedWait("b", 21*time.Second, f.at(-time.Hour))
	if m := f.read(30, nil).MedianWaitSeconds; *m.Value != 15 {
		t.Fatalf("median = %v", *m.Value)
	}
}

func TestMeasures_Agreement(t *testing.T) {
	f := newFixture(t)
	wantNull(t, "none", f.read(30, fakeAgreement{}).Agreement, coordinator.NullNoData)
	wantNull(t, "few", f.read(30, fakeAgreement{rated: 4, agreeing: 4}).Agreement, coordinator.NullTooFew)
	want(t, "five", f.read(30, fakeAgreement{rated: 5, agreeing: 4}).Agreement, 0.8, 4, 5)
}
