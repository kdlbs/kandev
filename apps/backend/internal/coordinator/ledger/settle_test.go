package ledger

import (
	"fmt"
	"testing"
	"time"
)

func (f *fixture) bulkTurns(n int, prefix string, startedAgo time.Duration, finished bool, snapshotHash string) {
	f.t.Helper()
	tx, err := f.db.Beginx()
	if err != nil {
		f.t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		var fin any
		if finished {
			fin = time.Now().UTC().Add(-startedAgo)
		}
		if _, err := tx.Exec(tx.Rebind(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", snapshot_hash, started_at, finished_at)
			VALUES (?, ?, ?, ?, 'message', ?, ?, ?)`), fmt.Sprintf("%s-%04d", prefix, i), f.coord.ID, "sess-"+prefix, fmt.Sprintf("%s-st-%04d", prefix, i),
			snapshotHash, time.Now().UTC().Add(-startedAgo), fin); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.db.Get(&n, f.db.Rebind(query), args...); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestSettle_ClosesRowsUnfinishedForADayInBatches(t *testing.T) {
	f := newFixture(t)
	f.bulkTurns(batchSize*2+7, "stuck", 25*time.Hour, false, "")
	f.bulkTurns(3, "fresh", 23*time.Hour, false, "")
	f.l.RunDaily(t.Context())

	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns WHERE outcome = 'interrupted' AND verdict = 'blocked' AND finished_at = started_at`); n != batchSize*2+7 {
		t.Fatalf("settled = %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns WHERE finished_at IS NULL`); n != 3 {
		t.Fatalf("fresh rows touched: %d unfinished", n)
	}
}

func TestSettle_DropsTheActiveEntryOfASettledRow(t *testing.T) {
	f := newFixture(t)
	f.start("st-1", time.Now().UTC().Add(-30*time.Hour))
	if f.l.ActiveTurnID(testSession) == "" {
		t.Fatal("no entry")
	}
	f.l.RunDaily(t.Context())
	if got := f.l.ActiveTurnID(testSession); got != "" {
		t.Fatalf("settled row left entry %q", got)
	}
}

func TestRetention_DeletesOldTurnsWithCallsInBatches(t *testing.T) {
	f := newFixture(t)
	day := 24 * time.Hour
	f.bulkTurns(batchSize+11, "old", 401*day, true, "")
	f.bulkTurns(2, "keep", 399*day, true, "")
	f.exec(`INSERT INTO coordinator_turn_calls (turn_id, action, allowed, recorded_at) VALUES ('old-0000', 'a', ?, ?), ('old-0510', 'a', ?, ?), ('keep-0000', 'a', ?, ?)`,
		true, f.now, true, f.now, true, f.now)
	f.l.RunDaily(t.Context())

	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns`); n != 2 {
		t.Fatalf("turns left = %d, want 2", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turn_calls`); n != 1 {
		t.Fatalf("calls left = %d, want 1 (only the kept turn's)", n)
	}
}

func TestRetention_SnapshotAgeIsTheNewestReferencingTurn(t *testing.T) {
	f := newFixture(t)
	day := 24 * time.Hour
	for _, h := range []string{"shared", "stale", "orphan"} {
		f.exec(`INSERT INTO coordinator_turn_snapshots (hash, body, created_at) VALUES (?, '{}', ?)`, h, time.Now().UTC().Add(-500*day))
	}
	f.bulkTurns(1, "recent", 80*day, true, "shared")
	f.bulkTurns(1, "ancient", 100*day, true, "shared")
	f.bulkTurns(1, "ancient2", 100*day, true, "stale")
	f.l.RunDaily(t.Context())

	var kept []string
	if err := f.db.Select(&kept, `SELECT hash FROM coordinator_turn_snapshots ORDER BY hash`); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(kept) != "[shared]" {
		t.Fatalf("snapshots kept = %v, want [shared]", kept)
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns`); n != 3 {
		t.Fatalf("turns under 400 days must stay: %d", n)
	}
}

func TestRetention_FailingBatchIsCountedAndTheJobEnds(t *testing.T) {
	f := newFixture(t)
	f.bulkTurns(2, "old", 401*24*time.Hour, true, "")
	f.exec(`ALTER TABLE coordinator_turn_calls RENAME TO coordinator_turn_calls_gone`)
	before := FailureCount(StageRetention)
	f.l.RunDaily(t.Context())
	if FailureCount(StageRetention) <= before {
		t.Fatal("retention failure not counted")
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns`); n != 2 {
		t.Fatalf("a failed batch must roll back: %d rows", n)
	}
}

func TestPass_CorrectsCeilingAndPauseOutcomesWithinTheWindow(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	mk := func(id string, finishedAgo time.Duration, model, linkedOutcome string) {
		f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", model, started_at, finished_at, outcome, verdict)
			VALUES (?, ?, 's', ?, 'wake', ?, ?, ?, 'cancelled', 'blocked')`, id, f.coord.ID, id, model, now.Add(-finishedAgo-time.Minute), now.Add(-finishedAgo))
		f.exec(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at, ledger_turn_id)
			VALUES (?, ?, 'c', 's', 1, 1, ?, ?, ?)`, "u-"+id, f.coord.ID, linkedOutcome, now, id)
	}
	mk("ceiling", time.Hour, "has-model", "stopped_at_ceiling")
	mk("pause", time.Hour, "", "stopped_by_pause")
	mk("completed-link", time.Hour, "", "completed")
	mk("too-old", 25*time.Hour, "", "stopped_at_ceiling")
	f.l.RunPass(t.Context())

	want := map[string]string{"ceiling": "stopped_at_ceiling", "pause": "stopped_by_pause", "completed-link": "cancelled", "too-old": "cancelled"}
	for id, outcome := range want {
		var got string
		if err := f.db.Get(&got, f.db.Rebind(`SELECT outcome FROM coordinator_turns WHERE id = ?`), id); err != nil || got != outcome {
			t.Fatalf("%s outcome = %q (%v), want %s", id, got, err, outcome)
		}
	}
	if n := f.count(`SELECT COUNT(*) FROM coordinator_turns WHERE verdict = 'blocked' AND outcome IN ('stopped_at_ceiling','stopped_by_pause')`); n != 2 {
		t.Fatalf("corrected rows = %d", n)
	}
}

func TestPass_FillsModelReportedAfterCompletionOnlyWhileEmpty(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	for _, r := range []struct{ id, model string }{{"empty", ""}, {"set", "kept"}} {
		f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", model, started_at, finished_at, outcome, verdict)
			VALUES (?, ?, 's', ?, 'message', ?, ?, ?, 'completed', 'nothing_needed')`, r.id, f.coord.ID, "st-"+r.id, r.model, now.Add(-time.Hour), now.Add(-time.Minute))
		f.exec(`INSERT INTO task_usage_events (session_id, turn_id, agent_type, model, created_at) VALUES ('s', ?, '', '', ?), ('s', ?, 'agent', ' Fresh-1 ', ?), ('s', ?, 'agent', 'second', ?)`,
			"st-"+r.id, now.Add(-2*time.Minute), "st-"+r.id, now.Add(-time.Minute), "st-"+r.id, now)
	}
	f.l.RunPass(t.Context())
	got := map[string]string{}
	for _, r := range f.turns() {
		got[r.ID] = r.Model + "|" + r.Harness
	}
	if got["empty"] != "fresh-1|agent@v9" || got["set"] != "kept|" {
		t.Fatalf("models = %v", got)
	}
}
