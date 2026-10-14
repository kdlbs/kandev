package recorder

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func (f *fixture) grader() *Grader { return NewGrader(f.db, zap.NewNop()) }

func (f *fixture) grade(id string, decidedAt time.Time) {
	f.t.Helper()
	if err := f.grader().Grade(context.Background(), id, decidedAt); err != nil {
		f.t.Fatalf("grade %s: %v", id, err)
	}
}

func TestGrade_ApprovedCreateTaskOpen(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.approvedRow("a1", "p1", "create_task", "t1", f.at(-time.Hour))
	f.grade("p1", f.at(-time.Hour))
	o := f.outcome("p1")
	if o == nil || o.Decision != "approved" || o.Final || o.TaskResult == nil || *o.TaskResult != "open" || o.LastStepID != "s1" {
		t.Fatalf("row = %+v", o)
	}
	if o.Cost != nil || o.ApprovedAt == nil {
		t.Fatalf("cost/approved_at = %+v", o)
	}
}

func TestGrade_NoDecisionNoRow(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "pending", "create_task", "", f.at(0))
	f.grade("p1", time.Time{})
	if f.outcomeCount() != 0 {
		t.Fatal("row for undecided proposal")
	}
}

func TestGrade_RejectedIsFinalWithReason(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	f.exec(`UPDATE coordinator_proposals SET reject_reason = 'duplicate' WHERE id = 'p1'`)
	f.grade("p1", time.Time{})
	o := f.outcome("p1")
	if o.Decision != "rejected" || !o.Final || o.ReasonCode != "other" {
		t.Fatalf("row = %+v", o)
	}
}

func TestGrade_EditedAndAutomatic(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.exec(`UPDATE coordinator_proposals SET final_spec_json = '{"title":"b","step_id":"s1"}', claimed_automatically = ? WHERE id = 'p1'`, true)
	f.grade("p1", time.Time{})
	o := f.outcome("p1")
	if o.Decision != "edited" || o.EditedJSON != `["title"]` || !o.Automatic {
		t.Fatalf("row = %+v", o)
	}
}

func TestGrade_UndoneOnFinalRowAndFrozenOtherwise(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.step("done", 2, true)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.approvedRow("a1", "p1", "create_task", "t1", f.at(-time.Hour))
	f.exec(`INSERT INTO github_task_prs (id, task_id, state, merged_at) VALUES ('a', 't1', 'merged', ?)`, f.now)
	f.grade("p1", time.Time{})
	if o := f.outcome("p1"); !o.Final || o.Decision != "approved" || *o.TaskResult != "merged" {
		t.Fatalf("row = %+v", o)
	}
	// A final row ignores later facts.
	f.exec(`UPDATE tasks SET workflow_step_id = 's1'`)
	f.exec(`DELETE FROM github_task_prs`)
	f.grade("p1", time.Time{})
	if o := f.outcome("p1"); *o.TaskResult != "merged" || !o.Final {
		t.Fatalf("final row moved: %+v", o)
	}
	f.exec(`UPDATE coordinator_activity SET undone_at = ? WHERE id = 'a1'`, f.now)
	f.grade("p1", time.Time{})
	if o := f.outcome("p1"); o.Decision != "undone" || !o.Final {
		t.Fatalf("undone not applied: %+v", o)
	}
}

func TestGrade_ReopenCountsOnce(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.step("done", 2, true)
	f.task("t1", "done", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.approvedRow("a1", "p1", "create_task", "t1", f.at(-time.Hour))
	// Seed a non-final done row, as a done result that can still reopen.
	f.grade("p1", time.Time{})
	f.exec(`UPDATE coordinator_outcomes SET final = ?, task_result = 'done'`, false)
	f.exec(`UPDATE tasks SET workflow_step_id = 's1'`)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = f.grader().Grade(context.Background(), "p1", time.Time{}) }()
	}
	wg.Wait()
	if o := f.outcome("p1"); o.ReopenCount != 1 || *o.TaskResult != "open" {
		t.Fatalf("row = %+v", o)
	}
}

func TestGrade_CostNullVersusZeroVersusUnpriced(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	if f.outcome("p1").Cost != nil {
		t.Fatal("no usage rows must give null cost")
	}
	f.exec(`INSERT INTO task_usage_events (task_id, cost_subcents) VALUES ('t1', 0)`)
	f.grade("p1", time.Time{})
	if c := f.outcome("p1").Cost; c == nil || *c != 0 {
		t.Fatalf("zero cost = %v", c)
	}
	f.exec(`INSERT INTO task_usage_events (task_id, cost_subcents, cost_source) VALUES ('t1', 5, 'unpriced')`)
	f.grade("p1", time.Time{})
	if f.outcome("p1").Cost != nil {
		t.Fatal("unpriced row must give null cost")
	}
}

func TestGrade_DeletedTaskIsFinalKeepingResult(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	f.exec(`DELETE FROM tasks`)
	f.grade("p1", time.Time{})
	if o := f.outcome("p1"); !o.Final || o.TaskResult == nil || *o.TaskResult != "open" {
		t.Fatalf("row = %+v", o)
	}
}

func TestGrade_MergedAtIsEarliestMergedPR(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.exec(`INSERT INTO github_task_prs (id, task_id, state, merged_at) VALUES ('a', 't1', 'merged', ?), ('b', 't1', 'merged', ?)`, f.at(-time.Minute), f.at(-2*time.Minute))
	f.grade("p1", time.Time{})
	o := f.outcome("p1")
	if o.MergedAt == nil || !o.MergedAt.Equal(f.at(-2*time.Minute)) || *o.TaskResult != "merged" || !o.Final {
		t.Fatalf("row = %+v", o)
	}
}

func TestGrade_FillsMissingTurnIDOnFinalRow(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	if f.outcome("p1").TurnID != nil {
		t.Fatal("unexpected turn id")
	}
	f.exec(`UPDATE coordinator_proposals SET turn_id = 'turn-1' WHERE id = 'p1'`)
	f.grade("p1", time.Time{})
	if o := f.outcome("p1"); o.TurnID == nil || *o.TurnID != "turn-1" {
		t.Fatalf("turn = %v", o.TurnID)
	}
}

func TestGrade_DecidedAtNeverRewritten(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.grade("p1", f.at(-time.Hour))
	first := f.outcome("p1").DecidedAt
	f.grade("p1", f.at(-time.Minute))
	if !f.outcome("p1").DecidedAt.Equal(first) {
		t.Fatal("decided_at rewritten")
	}
}

func TestGrade_ReadErrorKeepsRowAndCounts(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	before := GradeFailedCount(GradeTaskRead)
	f.exec(`DROP TABLE github_task_prs`)
	if err := f.grader().Grade(context.Background(), "p1", time.Time{}); err == nil {
		t.Fatal("expected error")
	}
	if GradeFailedCount(GradeTaskRead) != before+1 || f.outcome("p1") == nil {
		t.Fatal("read failure not counted or row lost")
	}
}

func TestGrade_DetachedMergedPRDoesNotMerge(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.exec(`INSERT INTO github_task_prs (id, task_id, state, merged_at, detached_at) VALUES ('a', 't1', 'merged', ?, ?)`, f.now, f.now)
	f.grade("p1", time.Time{})
	if r := f.outcome("p1"); r.MergedAt != nil || (r.TaskResult != nil && *r.TaskResult == "merged") || r.Final {
		t.Fatalf("detached PR counted: %+v", r)
	}
}
