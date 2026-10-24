package ledger

import (
	"context"
	"testing"
	"time"
)

func (f *fixture) dropTable(name string) {
	f.t.Helper()
	f.exec(`DROP TABLE ` + name)
}

func (f *fixture) finishedTurn() turnSnapshotRow {
	f.t.Helper()
	row := f.oneTurn()
	if row.FinishedAt == nil || row.Outcome == nil {
		f.t.Fatalf("turn not finished: %+v", row)
	}
	return row
}

func TestFailureStage_ModelCountedAndTurnStillFinishes(t *testing.T) {
	f := newFixture(t)
	f.dropTable("task_usage_events")
	before := FailureCount(StageModel)
	f.start("st-1", f.at(-time.Second))
	f.complete("st-1", f.at(-time.Second), f.at(0))
	f.finishedTurn()
	if got := FailureCount(StageModel) - before; got != 1 {
		t.Fatalf("model failures = %d, want 1", got)
	}
}

func TestFailureStage_LinkCountedAndTurnStillFinishes(t *testing.T) {
	f := newFixture(t)
	f.dropTable("coordinator_activity")
	before := FailureCount(StageLink)
	f.start("st-1", f.at(-time.Second))
	f.complete("st-1", f.at(-time.Second), f.at(0))
	f.finishedTurn()
	if got := FailureCount(StageLink) - before; got != 1 {
		t.Fatalf("link failures = %d, want 1", got)
	}
}

func TestFailureStage_CompleteCountedWhenTheConditionalUpdateFails(t *testing.T) {
	f := newFixture(t)
	f.start("st-1", f.at(-time.Second))
	f.exec(`CREATE TRIGGER block_finish BEFORE UPDATE OF finished_at ON coordinator_turns BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	before := FailureCount(StageComplete)
	f.complete("st-1", f.at(-time.Second), f.at(0))
	if got := FailureCount(StageComplete) - before; got != 1 {
		t.Fatalf("complete failures = %d, want 1", got)
	}
	if row := f.oneTurn(); row.FinishedAt != nil {
		t.Fatalf("row finished despite the blocked update: %+v", row)
	}
}

func TestFailureStage_CallCountedAndRecorderKeepsRunning(t *testing.T) {
	f := newFixture(t)
	f.startWriter()
	f.start("st-1", f.at(-time.Second))
	f.dropTable("coordinator_turn_calls")
	before := FailureCount(StageCall)
	f.l.Call(testSession, "list_activity", "", true)
	waitFor(t, func() bool { return FailureCount(StageCall) > before })
	if got := FailureCount(StageCall) - before; got < 1 {
		t.Fatalf("call failures = %d, want at least 1", got)
	}
}

func TestFailureStage_SettleCountedByThePassAndTheDailyJob(t *testing.T) {
	f := newFixture(t)
	f.dropTable("coordinator_unattended_turns")
	before := FailureCount(StageSettle)
	f.l.RunPass(context.Background())
	if got := FailureCount(StageSettle) - before; got != 1 {
		t.Fatalf("pass settle failures = %d, want 1", got)
	}
	f.l.RunDaily(context.Background())
	if FailureCount(StageSettle) != before+1 {
		t.Fatalf("daily settle must not fail when it reads only the turns table: %d", FailureCount(StageSettle)-before)
	}
}

func TestFailureStage_PassModelCountedWhenUsageTableIsMissing(t *testing.T) {
	f := newFixture(t)
	f.start("st-1", f.at(-time.Hour))
	f.complete("st-1", f.at(-time.Hour), f.at(-30*time.Minute))
	f.dropTable("task_usage_events")
	before := FailureCount(StageModel)
	f.l.RunPass(context.Background())
	if got := FailureCount(StageModel) - before; got != 1 {
		t.Fatalf("pass model failures = %d, want 1", got)
	}
}
