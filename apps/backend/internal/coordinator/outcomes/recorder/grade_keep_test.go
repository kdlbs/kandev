package recorder

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestGrade_DeletedTaskKeepsStoredCostStepAndMergeTime(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.exec(`INSERT INTO task_usage_events (task_id, cost_subcents) VALUES ('t1', 52000)`)
	f.grade("p1", time.Time{})
	before := f.outcome("p1")
	if before.Cost == nil || *before.Cost != 52000 || before.LastStepID != "s1" {
		t.Fatalf("seed row = %+v", before)
	}
	f.exec(`DELETE FROM tasks`)
	f.exec(`DELETE FROM task_usage_events`)
	f.grade("p1", time.Time{})
	after := f.outcome("p1")
	if !after.Final || after.Cost == nil || *after.Cost != 52000 || after.LastStepID != "s1" {
		t.Fatalf("deleted task lost stored values: %+v", after)
	}
}

func TestGrade_UndoOfFinalRowChangesOnlyTheDecision(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.step("done", 2, true)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.approvedRow("a1", "p1", "create_task", "t1", f.at(-time.Hour))
	f.exec(`INSERT INTO task_usage_events (task_id, cost_subcents) VALUES ('t1', 7)`)
	f.exec(`INSERT INTO github_task_prs (id, task_id, state, merged_at) VALUES ('a', 't1', 'merged', ?)`, f.at(-time.Minute))
	f.grade("p1", time.Time{})
	before := *f.outcome("p1")
	if !before.Final {
		t.Fatalf("seed not final: %+v", before)
	}
	f.exec(`UPDATE coordinator_activity SET undone_at = ? WHERE id = 'a1'`, f.now)
	f.exec(`DELETE FROM task_usage_events`)
	f.exec(`UPDATE tasks SET workflow_step_id = 's1'`)
	f.grade("p1", time.Time{})
	after := *f.outcome("p1")
	if after.Decision != "undone" {
		t.Fatalf("decision = %s", after.Decision)
	}
	after.Decision = before.Decision
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("undo changed more than the decision:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestGrade_EachReadFailureCountsKeepsRowAndHeals(t *testing.T) {
	cases := []struct {
		reason string
		drop   string
	}{
		{GradeProposalRead, "coordinator_activity"},
		{GradeUsageRead, "task_usage_events"},
	}
	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			f := newFixture(t)
			f.step("s1", 1, false)
			f.task("t1", "s1", false)
			f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
			f.grade("p1", time.Time{})
			before := *f.outcome("p1")
			n := GradeFailedCount(c.reason)
			f.exec(`ALTER TABLE ` + c.drop + ` RENAME TO ` + c.drop + `_gone`)
			if err := f.grader().Grade(context.Background(), "p1", time.Time{}); err == nil {
				t.Fatal("expected error")
			}
			if GradeFailedCount(c.reason) != n+1 {
				t.Fatalf("%s not counted", c.reason)
			}
			if got := *f.outcome("p1"); !reflect.DeepEqual(got, before) {
				t.Fatalf("row changed on failed read:\n%+v\n%+v", before, got)
			}
			f.exec(`ALTER TABLE ` + c.drop + `_gone RENAME TO ` + c.drop)
			f.grade("p1", time.Time{})
		})
	}
}

func TestSweep_StopsWhenContextEnds(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"p1", "p2", "p3"} {
		f.proposal(id, "rejected", "create_task", "", f.at(-time.Hour))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n := (SQLProposals{DB: f.db}).Sweep(ctx, f.queue()); n != 0 || f.outcomeCount() != 0 {
		t.Fatalf("graded %d, rows %d after cancel", n, f.outcomeCount())
	}
}
