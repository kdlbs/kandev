package recorder

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func (f *fixture) queue() *Queue { return NewQueue(f.grader(), zap.NewNop()) }

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !ok() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestQueue_DecisionHookGradesAndStopIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	q := f.queue()
	q.Start(context.Background())
	q.OnDecision(context.Background(), coordinator.DecisionEvent{ProposalID: "p1", At: f.at(-time.Hour)})
	waitFor(t, "grade", func() bool { return f.outcome("p1") != nil })
	q.Stop()
	q.Stop()
	q.Enqueue("p1", time.Time{}) // after stop: ignored
}

func TestQueue_DedupesAndCountsFull(t *testing.T) {
	f := newFixture(t)
	q := f.queue() // not started: nothing drains
	q.Enqueue("p1", time.Time{})
	q.Enqueue("p1", f.at(0))
	if len(q.order) != 1 || !q.pending["p1"].Equal(f.at(0)) {
		t.Fatalf("queue = %d, pending = %v", len(q.order), q.pending)
	}
	before := GradeFailedCount(GradeQueueFull)
	for i := 0; i < gradeQueueCap+1; i++ {
		q.Enqueue("x"+time.Duration(i).String(), time.Time{})
	}
	if GradeFailedCount(GradeQueueFull) <= before {
		t.Fatal("full queue not counted")
	}
}

func TestQueue_PanicInGradeIsRecovered(t *testing.T) {
	f := newFixture(t)
	q := NewQueue(NewGrader(nil, nil), nil) // nil db panics inside Grade
	before := ObserverPanicCount(ObserverGrader)
	q.gradeOne(context.Background(), "p1", time.Time{})
	if ObserverPanicCount(ObserverGrader) != before+1 {
		t.Fatal("panic not counted")
	}
	_ = f
}

func TestQueue_TaskEventGradesOpenProposals(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	b := bus.NewMemoryEventBus(logger.Default())
	q := f.queue()
	q.Start(context.Background())
	defer q.Stop()
	unsub, err := q.Subscribe(b, SQLProposals{DB: f.db})
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()
	if err := b.Publish(context.Background(), events.TaskMoved, bus.NewEvent(events.TaskMoved, "test", map[string]any{"task_id": "t1"})); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "grade by event", func() bool { return f.outcome("p1") != nil })
}

func TestSweep_GradesMissingAndOpenRowsOnce(t *testing.T) {
	f := newFixture(t)
	f.step("s1", 1, false)
	f.task("t1", "s1", false)
	f.proposal("p1", "approved", "create_task", "t1", f.at(-time.Hour))
	f.proposal("p2", "rejected", "create_task", "", f.at(-time.Hour))
	f.proposal("p3", "pending", "create_task", "", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	f.exec(`UPDATE coordinator_outcomes SET graded_at = ?`, f.at(-48*time.Hour))
	n := SQLProposals{DB: f.db}.Sweep(context.Background(), f.queue())
	if n != 2 || f.outcome("p2") == nil || f.outcome("p3") != nil {
		t.Fatalf("graded %d, p2=%v p3=%v", n, f.outcome("p2"), f.outcome("p3"))
	}
	if !f.outcome("p1").GradedAt.After(f.at(-time.Hour)) {
		t.Fatal("open row not regraded")
	}
}

type panicObserver struct{}

func (panicObserver) OnDecision(context.Context, coordinator.DecisionEvent) { panic("boom") }

func TestObservers_PanicInOneDoesNotStopTheOthers(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	q := f.queue()
	o := &Observers{log: zap.NewNop(), list: []namedObserver{{ObserverOverride, panicObserver{}}, {ObserverGrader, q}}}
	before := ObserverPanicCount(ObserverOverride)
	o.OnDecision(context.Background(), coordinator.DecisionEvent{ProposalID: "p1"})
	if ObserverPanicCount(ObserverOverride) != before+1 || len(q.order) != 1 {
		t.Fatal("panic not counted or later observer skipped")
	}
}

func TestSweep_SkipsProposalsOlderThanRetentionWindow(t *testing.T) {
	f := newFixture(t)
	f.proposal("old", "rejected", "create_task", "", f.at(-401*24*time.Hour))
	f.proposal("recent", "rejected", "create_task", "", f.at(-399*24*time.Hour))
	n := SQLProposals{DB: f.db}.Sweep(context.Background(), f.queue())
	if n != 1 || f.outcome("old") != nil || f.outcome("recent") == nil {
		t.Fatalf("graded %d, old=%v recent=%v", n, f.outcome("old"), f.outcome("recent"))
	}
}

func TestSweep_FillsEmptyTurnIDOnFinalRow(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(-time.Hour))
	f.grade("p1", time.Time{})
	if f.outcome("p1").TurnID != nil {
		t.Fatal("row stamped before the proposal had a turn")
	}
	f.exec(`UPDATE coordinator_proposals SET turn_id = 'turn-1' WHERE id = 'p1'`)
	SQLProposals{DB: f.db}.Sweep(context.Background(), f.queue())
	if got := f.outcome("p1").TurnID; got == nil || *got != "turn-1" {
		t.Fatalf("turn id = %v", got)
	}
}
