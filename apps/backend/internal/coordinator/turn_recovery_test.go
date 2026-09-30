package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (f *deliverFixture) withFinder(finder *fakeFinder) {
	f.svc.SetDeliveryDeps(f.reader, finder, f.sender)
}

func (f *deliverFixture) duties(t *testing.T) {
	t.Helper()
	if err := f.svc.turnDuties(context.Background(), f.c.ID); err != nil {
		t.Fatalf("turnDuties: %v", err)
	}
}

func (f *deliverFixture) assertOpen(t *testing.T, id string) {
	t.Helper()
	if outcome, _, _, _ := f.turnRow(t, id); outcome.Valid {
		t.Fatalf("turn %s outcome = %v, want open", id, outcome)
	}
}

func (f *deliverFixture) assertSettled(t *testing.T, id, want string) {
	t.Helper()
	if outcome, _, _, _ := f.turnRow(t, id); outcome.String != want {
		t.Fatalf("turn %s outcome = %q, want %q", id, outcome.String, want)
	}
}

func TestBackstop_RecordsTheMessageOfABoundTurn(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{msgs: map[string]*WakeMessage{"ut": {ID: "m-1", TurnID: "rt"}}}
	f.withFinder(finder)
	f.reader.turns = map[string]*TurnInfo{"st": {}}
	f.active.turns[ceilingSession] = "st"
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.duties(t)
	if _, _, _, msg := f.turnRow(t, "ut"); msg.String != "m-1" {
		t.Fatalf("message_id = %q, want m-1", msg.String)
	}
}

func TestBackstop_AFinderFindingNothingChangesNothing(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.reader.turns = map[string]*TurnInfo{"st": {}}
	f.active.turns[ceilingSession] = "st"
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.duties(t)
	if _, _, _, msg := f.turnRow(t, "ut"); msg.Valid {
		t.Fatalf("message_id = %v, want null", msg)
	}
	f.assertOpen(t, "ut")
}

func TestBackstop_UnboundTurnSettlesSendFailedOnlyAfterTwoMinutesAndNotBusy(t *testing.T) {
	cases := []struct {
		name    string
		age     time.Duration
		state   taskmodels.TaskSessionState
		stateEr error
		want    string
	}{
		{"too young", time.Minute, taskmodels.TaskSessionStateWaitingForInput, nil, ""},
		{"idle and old", 3 * time.Minute, taskmodels.TaskSessionStateWaitingForInput, nil, outcomeSendFailed},
		{"running", 3 * time.Minute, taskmodels.TaskSessionStateRunning, nil, ""},
		{"starting", 3 * time.Minute, taskmodels.TaskSessionStateStarting, nil, ""},
		{"unreadable state defers", 3 * time.Minute, taskmodels.TaskSessionStateWaitingForInput, errors.New("boom"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			f.withFinder(&fakeFinder{})
			f.reader.states[ceilingSession] = string(tc.state)
			f.reader.stateErr = tc.stateEr
			f.openTurnRow(t, openTurn{id: "ut", started: time.Now().UTC().Add(-tc.age)})
			f.duties(t)
			if tc.want == "" {
				f.assertOpen(t, "ut")
				return
			}
			f.assertSettled(t, "ut", tc.want)
			if status, turn := f.wakeStatus(t, "w-ut"); status != "pending" || turn.Valid {
				t.Fatalf("wake = %s/%v, want pending with no turn", status, turn)
			}
		})
	}
}

func TestBackstop_AnOrphanTurnIsCompletedBeforeTheSettleAndTheMessageIsMarked(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.openTurnRow(t, openTurn{id: "ut", reserved: "rt", message: "m-1", started: time.Now().UTC().Add(-3 * time.Minute)})
	f.duties(t)
	f.assertSettled(t, "ut", outcomeSendFailed)
	if len(finder.completed) != 1 || finder.completed[0] != [2]string{ceilingSession, "rt"} {
		t.Fatalf("completed = %v", finder.completed)
	}
	if len(finder.marked) != 1 || finder.marked[0] != [2]string{ceilingSession, "m-1"} {
		t.Fatalf("marked = %v", finder.marked)
	}
}

func TestBackstop_AFailedOrphanCompletionLeavesTheRowUntouched(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{completeErr: errors.New("busy")}
	f.withFinder(finder)
	f.openTurnRow(t, openTurn{id: "ut", reserved: "rt", started: time.Now().UTC().Add(-3 * time.Minute)})
	f.duties(t)
	f.assertOpen(t, "ut")
	if status, _ := f.wakeStatus(t, "w-ut"); status != "delivered" {
		t.Fatalf("wake = %s, want delivered", status)
	}
}

func TestBackstop_ANullOrEmptyReservedIdCompletesNothing(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.openTurnRow(t, openTurn{id: "ut", started: time.Now().UTC().Add(-3 * time.Minute)})
	mustExec(t, f.store, `UPDATE coordinator_unattended_turns SET reserved_turn_id = '' WHERE id = 'ut'`)
	f.duties(t)
	f.assertSettled(t, "ut", outcomeSendFailed)
	if len(finder.completed) != 0 {
		t.Fatalf("completed = %v, want none", finder.completed)
	}
}

func TestBackstop_MissedSettleIsRederivedOnlyForBoundRows(t *testing.T) {
	cases := []struct {
		name   string
		turn   *TurnInfo
		active string
		turnEr error
		want   string
	}{
		{"turn gone", nil, "st", nil, outcomeCompleted},
		{"turn completed", &TurnInfo{Completed: true}, "st", nil, outcomeCompleted},
		{"active turn is another", &TurnInfo{}, "other", nil, outcomeCompleted},
		{"no active turn", &TurnInfo{}, "", nil, outcomeCompleted},
		{"still the active turn", &TurnInfo{}, "st", nil, ""},
		{"turn read fails", nil, "", errors.New("boom"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			f.withFinder(&fakeFinder{})
			f.reader.turns = map[string]*TurnInfo{}
			if tc.turn != nil {
				f.reader.turns["st"] = tc.turn
			}
			f.reader.turnErr = tc.turnEr
			if tc.active != "" {
				f.active.turns[ceilingSession] = tc.active
			}
			f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st", message: "m"})
			f.duties(t)
			if tc.want == "" {
				f.assertOpen(t, "ut")
				return
			}
			f.assertSettled(t, "ut", tc.want)
		})
	}
}

func TestBackstop_AnActiveTurnReadFailureLeavesTheBoundRowOpen(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.reader.turns = map[string]*TurnInfo{"st": {}}
	f.active.err = errors.New("boom")
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st", message: "m"})
	_ = f.svc.turnDuties(context.Background(), f.c.ID)
	f.assertOpen(t, "ut")
}

func TestBackstop_RecomputesTheCostOfRecentlySettledTurnsOnly(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	now := time.Now().UTC()
	for id, age := range map[string]time.Duration{"recent": 2 * time.Minute, "old": 30 * time.Minute} {
		mustExec(t, f.store, `INSERT INTO coordinator_unattended_turns
			(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at, outcome, finished_at)
			VALUES (?, ?, ?, ?, ?, 1, 500, ?, 'completed', ?)`,
			id, f.c.ID, ceilingConvTask, ceilingSession, "st-"+id, now.Add(-time.Hour), now.Add(-age))
	}
	f.duties(t)
	if _, _, cost, _ := f.turnRow(t, "recent"); cost.String != "55" {
		t.Fatalf("recent cost = %v, want 55", cost)
	}
	if _, _, cost, _ := f.turnRow(t, "old"); cost.Valid {
		t.Fatalf("old cost = %v, want untouched", cost)
	}
}

func TestBackstop_RunsTheCeilingCheckForAnOpenBoundTurn(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.reader.turns = map[string]*TurnInfo{"st": {}}
	f.active.turns[ceilingSession] = "st"
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 5000}, nil }
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st", message: "m"})
	f.duties(t)
	if got := f.canceller.callCount(); got != 1 {
		t.Fatalf("cancel calls = %d, want 1", got)
	}
}

func TestStartupPass_SettlesAnUnboundPreT0RowInterrupted(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.openTurnRow(t, openTurn{id: "unbound", started: time.Now().UTC().Add(-time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertSettled(t, "unbound", outcomeInterrupted)
}

func TestStartupPass_LeavesABoundRowOpen(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.openTurnRow(t, openTurn{id: "bound", sessionTurn: "st", started: time.Now().UTC().Add(-time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertOpen(t, "bound")
}

func TestStartupPass_LeavesARowStartedAfterT0Open(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.openTurnRow(t, openTurn{id: "later", started: time.Now().UTC().Add(time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertOpen(t, "later")
}

func TestStartupPass_ARunningSessionWithAReservedTurnIsLeftForTheBackstop(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.reader.states[ceilingSession] = string(taskmodels.TaskSessionStateRunning)
	f.openTurnRow(t, openTurn{id: "ut", reserved: "rt", started: time.Now().UTC().Add(-time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertOpen(t, "ut")
	if len(finder.completed) != 0 {
		t.Fatalf("completed = %v, want none", finder.completed)
	}
}

func TestStartupPass_ANotBusySessionCompletesTheOrphanTurnThenSettles(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.openTurnRow(t, openTurn{id: "ut", reserved: "rt", started: time.Now().UTC().Add(-time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertSettled(t, "ut", outcomeInterrupted)
	if len(finder.completed) != 1 || finder.completed[0] != [2]string{ceilingSession, "rt"} {
		t.Fatalf("completed = %v", finder.completed)
	}
}

func TestStartupPass_ANullReservedTurnIsSettledEvenWithARunningSession(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.reader.states[ceilingSession] = string(taskmodels.TaskSessionStateRunning)
	f.openTurnRow(t, openTurn{id: "ut", started: time.Now().UTC().Add(-time.Hour)})
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.assertSettled(t, "ut", outcomeInterrupted)
	if len(finder.completed) != 0 {
		t.Fatalf("completed = %v, want none", finder.completed)
	}
}

func TestStartupPass_APrunedRowIsNotResurrectedAndPruneRuns(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	old := time.Now().UTC().Add(-400 * 24 * time.Hour)
	mustExec(t, f.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at, outcome, finished_at)
		VALUES ('ancient', ?, ?, ?, 1, 500, ?, 'completed', ?)`, f.c.ID, ceilingConvTask, ceilingSession, old, old)
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_unattended_turns WHERE id = 'ancient'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ancient turn rows = %d (%v), want pruned", n, err)
	}
}
