package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeActiveTurns struct {
	mu    sync.Mutex
	turns map[string]string
	err   error
	calls int
}

func (f *fakeActiveTurns) GetActiveTurn(_ context.Context, sessionID string) (*taskmodels.Turn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if id := f.turns[sessionID]; id != "" {
		return &taskmodels.Turn{ID: id, TaskSessionID: sessionID}, nil
	}
	return nil, nil
}

type fakeCanceller struct {
	mu    sync.Mutex
	calls [][2]string
	fn    func(ctx context.Context, sessionID, turnID string) error
}

func (f *fakeCanceller) CancelTurn(ctx context.Context, sessionID, turnID string) error {
	f.mu.Lock()
	f.calls = append(f.calls, [2]string{sessionID, turnID})
	fn := f.fn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, sessionID, turnID)
	}
	return nil
}

func (f *fakeCanceller) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type ceilingFixture struct {
	env       *phase3Env
	conv      *fakeConversationTasks
	ledger    *fakeSpendLedger
	turns     *fakeActiveTurns
	canceller *fakeCanceller
	window    int64
	turnCost  int64
}

const (
	ceilingSession     = "sess-1"
	ceilingSessionTurn = "st-1"
	ceilingTurnRow     = "ut-1"
	ceilingConvTask    = "conv-1"
)

func newCeilingFixture(t *testing.T, ceiling int64) *ceilingFixture {
	t.Helper()
	env := newPhase3Env(t, true)
	f := &ceilingFixture{
		env: env, conv: newFakeConversationTasks(), ledger: &fakeSpendLedger{},
		turns:     &fakeActiveTurns{turns: map[string]string{ceilingSession: ceilingSessionTurn}},
		canceller: &fakeCanceller{},
		turnCost:  55,
	}
	f.conv.tasks[ceilingConvTask] = spendConvTask(ceilingConvTask, env.c.ID, false)
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) {
		return taskmodels.UsageSum{CostSubcents: f.window}, nil
	}
	f.ledger.turnFn = func(turnCall) (int64, error) { return f.turnCost, nil }
	env.svc.conversationTasks = f.conv
	env.svc.SetSpendDeps(f.ledger, f.turns, f.canceller)
	mustExec(t, env.store, `UPDATE coordinators SET cost_ceiling_subcents = ? WHERE id = ?`, ceiling, env.c.ID)
	f.insertTurn(t, ceilingTurnRow, ceilingSession, ceilingSessionTurn, 500)
	return f
}

func (f *ceilingFixture) insertTurn(t *testing.T, id, sessionID, sessionTurnID string, startCeiling int64) {
	t.Helper()
	var turnArg any
	if sessionTurnID != "" {
		turnArg = sessionTurnID
	}
	mustExec(t, f.env.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		id, f.env.c.ID, ceilingConvTask, sessionID, turnArg, startCeiling, time.Now().UTC())
}

type turnRowState struct {
	Outcome    sql.NullString
	Cost       sql.NullInt64
	StopReq    sql.NullTime
	FinishedAt sql.NullTime
}

func (f *ceilingFixture) row(t *testing.T, id string) turnRowState {
	t.Helper()
	var r turnRowState
	err := f.env.store.db.QueryRow(f.env.store.db.Rebind(
		`SELECT outcome, cost_subcents, stop_requested_at, finished_at FROM coordinator_unattended_turns WHERE id = ?`), id).
		Scan(&r.Outcome, &r.Cost, &r.StopReq, &r.FinishedAt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type counterMark struct {
	name, key string
	base      int64
}

func mark(name, key string) counterMark { return counterMark{name, key, spendCounterValue(name, key)} }
func (m counterMark) delta() int64      { return spendCounterValue(m.name, m.key) - m.base }

func TestCheckCeiling_AtCeilingMarksCancelsOnceAndSettles(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	stops := mark("coordinator_ceiling_stop_total", "ceiling")
	settled := mark("coordinator_unattended_turn_total", "stopped_at_ceiling")

	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 || f.canceller.calls[0] != [2]string{ceilingSession, ceilingSessionTurn} {
		t.Fatalf("cancel calls = %v", f.canceller.calls)
	}
	r := f.row(t, ceilingTurnRow)
	if r.Outcome.String != "stopped_at_ceiling" || !r.StopReq.Valid || !r.FinishedAt.Valid || r.Cost.Int64 != 55 || !r.Cost.Valid {
		t.Fatalf("row = %+v", r)
	}
	if stops.delta() != 1 || settled.delta() != 1 {
		t.Fatalf("stop delta %d settled delta %d", stops.delta(), settled.delta())
	}
	if len(f.env.kicks) != 1 || f.env.kicks[0] != f.env.c.ID {
		t.Fatalf("kicks = %v", f.env.kicks)
	}
	waitForEvents(t, &f.env.events, 1)
	if ev := f.env.events.snapshot()[0]; !ev.AutonomyChanged {
		t.Fatalf("event = %+v, want autonomy_changed", ev)
	}
}

func waitForEvents(t *testing.T, c *captureCoordinatorUpdated, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(c.snapshot()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("got %d coordinator.updated events, want %d", len(c.snapshot()), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestCheckCeiling_BelowCeilingDoesNothing(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 999
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 0 {
		t.Fatal("cancelled below the ceiling")
	}
	if r := f.row(t, ceilingTurnRow); r.StopReq.Valid || r.Outcome.Valid {
		t.Fatalf("row = %+v", r)
	}
}

func TestCheckCeiling_UnmeasurableSpendStopsWithUnmeasurableReason(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, errors.New("db down") }
	stops := mark("coordinator_ceiling_stop_total", "unmeasurable")
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 || f.row(t, ceilingTurnRow).Outcome.String != "stopped_at_ceiling" {
		t.Fatalf("calls %d row %+v", f.canceller.callCount(), f.row(t, ceilingTurnRow))
	}
	if stops.delta() != 1 {
		t.Fatalf("unmeasurable stop delta = %d", stops.delta())
	}
}

func TestCheckCeiling_DegradedSpendStopsToo(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) {
		return taskmodels.UsageSum{CostSubcents: 1, HasUnpriced: true}, nil
	}
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 {
		t.Fatalf("cancel calls = %d", f.canceller.callCount())
	}
}

func TestCheckCeiling_ClearedCeilingFallsBackToTheTurnsStartCeiling(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `UPDATE coordinators SET cost_ceiling_subcents = NULL WHERE id = ?`, f.env.c.ID)
	f.window = 500 // start_ceiling_subcents of the row
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 {
		t.Fatalf("cancel calls = %d, want the start ceiling to apply", f.canceller.callCount())
	}
}

func TestCheckCeiling_NeverActsOnATurnWithoutASessionTurnID(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET session_turn_id = NULL WHERE id = ?`, ceilingTurnRow)
	f.window = 5000
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 0 || f.row(t, ceilingTurnRow).StopReq.Valid {
		t.Fatal("a turn with no session turn id was marked or cancelled")
	}
}

func TestCheckCeiling_AttendedTurnOverTheCeilingIsNeverCancelled(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 5000
	f.turns.turns[ceilingSession] = "st-manager"
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 0 || f.row(t, ceilingTurnRow).StopReq.Valid {
		t.Fatal("a manager's turn was marked or cancelled")
	}
	delete(f.turns.turns, ceilingSession)
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil || f.canceller.callCount() != 0 {
		t.Fatalf("no active turn: err %v cancels %d", err, f.canceller.callCount())
	}
}

func TestCheckCeiling_NoOpenTurnIsNothingToDo(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 5000
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET outcome = 'completed', finished_at = ? WHERE id = ?`, time.Now().UTC(), ceilingTurnRow)
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil || f.canceller.callCount() != 0 {
		t.Fatalf("err %v cancels %d", err, f.canceller.callCount())
	}
}

func TestCheckCeiling_FailedActiveTurnReadMarksNothing(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 5000
	f.turns.err = errors.New("read failed")
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err == nil {
		t.Fatal("want the read error")
	}
	if f.row(t, ceilingTurnRow).StopReq.Valid || f.canceller.callCount() != 0 {
		t.Fatal("marked or cancelled after a failed active-turn read")
	}
}

func TestCheckCeiling_CoordinatorGoneReturnsNil(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 5000
	mustExec(t, f.env.store, `DELETE FROM coordinators WHERE id = ?`, f.env.c.ID)
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil || f.canceller.callCount() != 0 {
		t.Fatalf("err %v cancels %d", err, f.canceller.callCount())
	}
}

func TestCheckCeiling_FailedCancelLeavesMarkedRowAndNextCallRetriesWhateverSpendReads(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	stops := mark("coordinator_ceiling_stop_total", "ceiling")
	failed := mark("coordinator_ceiling_cancel_failed_total", "")
	f.canceller.fn = func(context.Context, string, string) error { return errors.New("agentctl unreachable") }

	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err == nil {
		t.Fatal("want the cancel error")
	}
	r := f.row(t, ceilingTurnRow)
	if !r.StopReq.Valid || r.Outcome.Valid {
		t.Fatalf("row = %+v, want marked and open", r)
	}
	if failed.delta() != 1 {
		t.Fatalf("failed cancel delta = %d", failed.delta())
	}

	f.window = 0
	f.canceller.mu.Lock()
	f.canceller.fn = nil
	f.canceller.mu.Unlock()
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 2 || f.row(t, ceilingTurnRow).Outcome.String != "stopped_at_ceiling" {
		t.Fatalf("calls %d row %+v", f.canceller.callCount(), f.row(t, ceilingTurnRow))
	}
	if stops.delta() != 1 {
		t.Fatalf("stop counted %d times across the retry, want once", stops.delta())
	}
}

func TestCheckCeiling_CancelInFlightCountsAsFailedButTurnNotActiveDoesNot(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	failed := mark("coordinator_ceiling_cancel_failed_total", "")

	f.canceller.fn = func(context.Context, string, string) error { return orchestrator.ErrCancelInFlight }
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); !errors.Is(err, orchestrator.ErrCancelInFlight) {
		t.Fatalf("err = %v", err)
	}
	if failed.delta() != 1 {
		t.Fatalf("in-flight failed delta = %d", failed.delta())
	}

	f.canceller.mu.Lock()
	f.canceller.fn = func(context.Context, string, string) error { return orchestrator.ErrTurnNotActive }
	f.canceller.mu.Unlock()
	settled := mark("coordinator_unattended_turn_total", "stopped_at_ceiling")
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatalf("ErrTurnNotActive must return nil, got %v", err)
	}
	if failed.delta() != 1 || settled.delta() != 0 {
		t.Fatalf("failed delta %d settled delta %d", failed.delta(), settled.delta())
	}
	if r := f.row(t, ceilingTurnRow); !r.StopReq.Valid || r.Outcome.Valid {
		t.Fatalf("row = %+v, want the marked row left open", r)
	}
	if len(f.env.kicks) != 0 {
		t.Fatalf("kicks = %v, nothing settled", f.env.kicks)
	}
}

func TestCheckCeiling_ConcurrentCallsCancelOnce(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	entered := make(chan struct{})
	release := make(chan struct{})
	f.canceller.fn = func(context.Context, string, string) error {
		close(entered)
		<-release
		return nil
	}
	first := make(chan error, 1)
	go func() { first <- f.env.svc.CheckCeiling(context.Background(), f.env.c.ID) }()
	<-entered
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatalf("second call = %v, want nil at once", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 {
		t.Fatalf("CancelTurn called %d times, want 1", f.canceller.callCount())
	}
}

func TestCheckCeiling_TurnEndSettlingBetweenCancelAndSettleChangesTheRowOnce(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	settled := mark("coordinator_unattended_turn_total", "stopped_at_ceiling")
	f.canceller.fn = func(ctx context.Context, _, _ string) error {
		changed, err := f.env.store.settleTurnStoppedAtCeiling(ctx, ceilingTurnRow, time.Now().UTC(), nil)
		if err != nil || !changed {
			t.Errorf("stand-in settle changed=%v err=%v", changed, err)
		}
		return nil
	}
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); r.Outcome.String != "stopped_at_ceiling" || r.Cost.Valid {
		t.Fatalf("row = %+v, the first settle owns the row", r)
	}
	if settled.delta() != 0 || len(f.env.kicks) != 0 || len(f.env.events.snapshot()) != 0 {
		t.Fatalf("the losing settle counted/kicked/published: delta %d kicks %v events %d",
			settled.delta(), f.env.kicks, len(f.env.events.snapshot()))
	}
}

func TestCheckCeiling_SettlesWithNullCostWhenTheCostReadFails(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 1000
	f.ledger.turnFn = func(turnCall) (int64, error) { return 0, errors.New("boom") }
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); r.Outcome.String != "stopped_at_ceiling" || r.Cost.Valid {
		t.Fatalf("row = %+v", r)
	}
}

func TestCheckCeilingForSession(t *testing.T) {
	t.Run("stops the coordinator's turn on its session", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		if err := f.env.svc.CheckCeilingForSession(t.Context(), ceilingConvTask, ceilingSession); err != nil {
			t.Fatal(err)
		}
		if f.canceller.callCount() != 1 {
			t.Fatalf("cancel calls = %d", f.canceller.callCount())
		}
	})
	t.Run("another session is not the open turn", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		if err := f.env.svc.CheckCeilingForSession(t.Context(), ceilingConvTask, "sess-other"); err != nil || f.canceller.callCount() != 0 {
			t.Fatalf("err %v cancels %d", err, f.canceller.callCount())
		}
	})
	t.Run("a task naming no coordinator is ignored", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.conv.tasks["plain"] = spendConvTask("plain", "", false)
		if err := f.env.svc.CheckCeilingForSession(t.Context(), "plain", ceilingSession); err != nil || f.canceller.callCount() != 0 {
			t.Fatalf("err %v cancels %d", err, f.canceller.callCount())
		}
	})
	t.Run("a failed task read is returned and cancels nothing", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.conv.getErr = errors.New("read failed")
		if err := f.env.svc.CheckCeilingForSession(t.Context(), ceilingConvTask, ceilingSession); err == nil || f.canceller.callCount() != 0 {
			t.Fatalf("err %v cancels %d", err, f.canceller.callCount())
		}
	})
}

func TestObserveUsage(t *testing.T) {
	t.Run("logs a failed task read and ends the call", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.conv.getErr = errors.New("read failed")
		f.env.svc.ObserveUsage(t.Context(), ceilingConvTask, ceilingSession)
		if f.canceller.callCount() != 0 {
			t.Fatal("cancelled after a failed task read")
		}
	})
	t.Run("ignores empty ids", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.env.svc.ObserveUsage(t.Context(), "", ceilingSession)
		f.env.svc.ObserveUsage(t.Context(), ceilingConvTask, "")
		if f.canceller.callCount() != 0 {
			t.Fatal("acted on an empty id")
		}
	})
	t.Run("is inert when phase 3 is not effective", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.env.svc.phase3 = false
		f.env.svc.ObserveUsage(t.Context(), ceilingConvTask, ceilingSession)
		if f.canceller.callCount() != 0 {
			t.Fatal("acted with phase 3 off")
		}
	})
	t.Run("stops the turn on a notice", func(t *testing.T) {
		f := newCeilingFixture(t, 1000)
		f.window = 1000
		f.env.svc.ObserveUsage(t.Context(), ceilingConvTask, ceilingSession)
		if f.canceller.callCount() != 1 {
			t.Fatalf("cancel calls = %d", f.canceller.callCount())
		}
	})
}

func TestRecomputeTurnCost(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	finished := time.Now().UTC()
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET outcome = 'completed', finished_at = ? WHERE id = ?`, finished, ceilingTurnRow)
	key := TurnKey{SessionID: ceilingSession, SessionTurnID: ceilingSessionTurn, FinishedAt: finished}

	f.turnCost = 40
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, key); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); !r.Cost.Valid || r.Cost.Int64 != 40 {
		t.Fatalf("NULL cost not filled: %+v", r)
	}
	f.turnCost = 10
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, key); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); r.Cost.Int64 != 40 {
		t.Fatalf("recompute lowered the cost to %d", r.Cost.Int64)
	}
	f.turnCost = 90
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, key); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); r.Cost.Int64 != 90 {
		t.Fatalf("recompute did not raise the cost: %d", r.Cost.Int64)
	}
}

func TestRecomputeTurnCost_OpenTurnAndUnknownKeyAreLeftAlone(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	key := TurnKey{SessionID: ceilingSession, SessionTurnID: ceilingSessionTurn, FinishedAt: time.Now().UTC()}
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, key); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ceilingTurnRow); r.Cost.Valid {
		t.Fatalf("an open turn got a cost: %+v", r)
	}
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, TurnKey{}); err != nil {
		t.Fatalf("unknown key = %v", err)
	}
	f.ledger.turnFn = func(turnCall) (int64, error) { return 0, errors.New("boom") }
	if err := f.env.svc.RecomputeTurnCost(t.Context(), ceilingTurnRow, key); err == nil {
		t.Fatal("a failed read must be returned so the backstop retries")
	}
}
