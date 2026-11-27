package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
)

func (f *ceilingFixture) pause(t *testing.T) {
	t.Helper()
	if _, err := f.env.store.SetPaused(context.Background(), f.env.c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
}

func (f *ceilingFixture) linkWake(t *testing.T, wakeID, turnID string) {
	t.Helper()
	insertWakeRow(t, f.env.store, f.env.c, wakeID, "task-"+wakeID, string(WakeKindQuestion), "q-"+wakeID, "delivered", time.Now().UTC())
	mustExec(t, f.env.store, `UPDATE coordinator_wakes SET turn_id = ? WHERE id = ?`, turnID, wakeID)
}

func (f *ceilingFixture) wakeState(t *testing.T, id string) (string, sql.NullString) {
	t.Helper()
	var status string
	var turn sql.NullString
	err := f.env.store.db.QueryRow(f.env.store.db.Rebind(`SELECT status, turn_id FROM coordinator_wakes WHERE id = ?`), id).Scan(&status, &turn)
	if err != nil {
		t.Fatal(err)
	}
	return status, turn
}

func (f *ceilingFixture) marks(t *testing.T, id string) (requested, cancel sql.NullTime) {
	t.Helper()
	err := f.env.store.db.QueryRow(f.env.store.db.Rebind(
		`SELECT pause_requested_at, pause_cancel_at FROM coordinator_unattended_turns WHERE id = ?`), id).Scan(&requested, &cancel)
	if err != nil {
		t.Fatal(err)
	}
	return requested, cancel
}

func TestStop_RunningTurnIsCancelledSettledStoppedByPauseAndWakesReturn(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.linkWake(t, "w1", ceilingTurnRow)
	f.pause(t)
	settled := mark("coordinator_unattended_turn_total", "stopped_by_pause")

	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 1 || f.canceller.calls[0] != [2]string{ceilingSession, ceilingSessionTurn} {
		t.Fatalf("cancel calls = %v", f.canceller.calls)
	}
	r := f.row(t, ceilingTurnRow)
	if r.Outcome.String != "stopped_by_pause" || !r.FinishedAt.Valid || r.StopReq.Valid {
		t.Fatalf("row = %+v", r)
	}
	if status, turn := f.wakeState(t, "w1"); status != "pending" || turn.Valid {
		t.Fatalf("wake = %s/%v, want pending and unbound", status, turn)
	}
	if settled.delta() != 1 {
		t.Fatalf("stopped_by_pause delta = %d", settled.delta())
	}
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil || f.canceller.callCount() != 1 {
		t.Fatalf("second stop err=%v cancels=%d, want a no-op", err, f.canceller.callCount())
	}
}

func TestStop_AfterResumeCancelsNothing(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	if _, err := f.env.store.SetPaused(t.Context(), f.env.c.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	requested, _ := f.marks(t, ceilingTurnRow)
	if f.canceller.callCount() != 0 || requested.Valid || f.row(t, ceilingTurnRow).Outcome.Valid {
		t.Fatalf("a delayed stop acted after Resume: cancels=%d requested=%v", f.canceller.callCount(), requested)
	}
}

func TestStop_TurnNotActiveSettlesNothingAndKeepsTheIntent(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	f.canceller.fn = func(context.Context, string, string) error { return orchestrator.ErrTurnNotActive }
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	_, cancel := f.marks(t, ceilingTurnRow)
	if f.row(t, ceilingTurnRow).Outcome.Valid || !cancel.Valid {
		t.Fatalf("row = %+v cancel=%v", f.row(t, ceilingTurnRow), cancel)
	}
}

func TestStop_CancelInFlightIsRetriedOnTheNextPass(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	f.canceller.fn = func(context.Context, string, string) error { return orchestrator.ErrCancelInFlight }
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatalf("in-flight cancel is not an error: %v", err)
	}
	if f.row(t, ceilingTurnRow).Outcome.Valid {
		t.Fatal("settled while the cancel was in flight")
	}
	f.canceller.fn = nil
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.row(t, ceilingTurnRow).Outcome.String != "stopped_by_pause" || f.canceller.callCount() != 2 {
		t.Fatalf("row = %+v cancels=%d", f.row(t, ceilingTurnRow), f.canceller.callCount())
	}
}

func TestStop_AFailedCancelDoesNotSkipTheDreamStop(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	boom := errors.New("cancel exploded")
	f.canceller.fn = func(context.Context, string, string) error { return boom }
	dreams := 0
	f.env.svc.SetDreamStop(func(context.Context, string) error { dreams++; return nil })
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the joined cancel failure", err)
	}
	if dreams != 1 || f.row(t, ceilingTurnRow).Outcome.Valid {
		t.Fatalf("dream stops = %d row = %+v", dreams, f.row(t, ceilingTurnRow))
	}
}

func TestStop_CeilingWinsOverPauseAndPauseAloneIsNeverACeilingStop(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.window = 10
	_, _ = f.env.store.markPauseCancel(t.Context(), ceilingTurnRow)
	if err := f.env.svc.CheckCeiling(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.canceller.callCount() != 0 || f.row(t, ceilingTurnRow).StopReq.Valid {
		t.Fatal("pause alone made the ceiling stop the turn")
	}
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET stop_requested_at = ? WHERE id = ?`, time.Now().UTC(), ceilingTurnRow)
	turn, err := f.env.store.getUnattendedTurn(t.Context(), ceilingTurnRow)
	if err != nil {
		t.Fatal(err)
	}
	f.env.svc.settleBoundTurn(t.Context(), turn)
	if got := f.row(t, ceilingTurnRow).Outcome.String; got != "stopped_at_ceiling" {
		t.Fatalf("outcome = %q, want the ceiling to win", got)
	}
}

func TestTurnEnd_FinishingOnItsOwnAfterTheMarkIsCompleted(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.linkWake(t, "w1", ceilingTurnRow)
	f.env.svc.convReader = &fakeConvReader{states: map[string]string{ceilingSession: "WAITING_FOR_INPUT"}}
	if _, err := f.env.store.markPauseRequested(t.Context(), ceilingTurnRow); err != nil {
		t.Fatal(err)
	}
	turn, err := f.env.store.getUnattendedTurn(t.Context(), ceilingTurnRow)
	if err != nil {
		t.Fatal(err)
	}
	f.env.svc.settleBoundTurn(t.Context(), turn)
	if got := f.row(t, ceilingTurnRow).Outcome.String; got != "completed" {
		t.Fatalf("outcome = %q, want completed", got)
	}
	if status, _ := f.wakeState(t, "w1"); status != "delivered" {
		t.Fatalf("wake = %s, want it left handled", status)
	}
}

func TestStop_UnsentRowWaitsTwoMinutesThenSettlesAndReturnsWakes(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-u", ceilingSession, "", 500)
	f.linkWake(t, "w1", "ut-u")
	f.pause(t)
	started := time.Now().UTC()
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET started_at = ? WHERE id = ?`, started, "ut-u")

	f.env.store.now = func() time.Time { return started.Add(sendSettleAge - time.Second) }
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	requested, _ := f.marks(t, "ut-u")
	if f.row(t, "ut-u").Outcome.Valid || !requested.Valid {
		t.Fatalf("at 1m59s: row = %+v requested=%v", f.row(t, "ut-u"), requested)
	}
	f.env.store.now = func() time.Time { return started.Add(sendSettleAge) }
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.row(t, "ut-u").Outcome.String != "stopped_by_pause" {
		t.Fatalf("at 2m: row = %+v", f.row(t, "ut-u"))
	}
	if status, turn := f.wakeState(t, "w1"); status != "pending" || turn.Valid {
		t.Fatalf("wake = %s/%v", status, turn)
	}
	if f.canceller.callCount() != 0 {
		t.Fatal("cancelled a turn that was never bound")
	}
}

func TestStop_ReservedRowIsMarkedNotSettled(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-r", ceilingSession, "", 500)
	old := time.Now().UTC().Add(-time.Hour)
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET reserved_turn_id = 'rt-1', started_at = ? WHERE id = ?`, old, "ut-r")
	f.pause(t)
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	requested, _ := f.marks(t, "ut-r")
	if f.row(t, "ut-r").Outcome.Valid || !requested.Valid {
		t.Fatalf("row = %+v requested=%v", f.row(t, "ut-r"), requested)
	}
}

func TestSettlePausedUnsentTurn_ReservationOrResumeWinsTheRace(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-u", ceilingSession, "", 500)
	f.pause(t)
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET reserved_turn_id = 'rt-1' WHERE id = 'ut-u'`)
	if changed, err := f.env.store.settlePausedUnsentTurn(t.Context(), "ut-u", f.env.c.ID); err != nil || changed {
		t.Fatalf("reserved row settled: %v %v", changed, err)
	}
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET reserved_turn_id = NULL WHERE id = 'ut-u'`)
	if _, err := f.env.store.SetPaused(t.Context(), f.env.c.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.env.store.settlePausedUnsentTurn(t.Context(), "ut-u", f.env.c.ID); err != nil || changed {
		t.Fatalf("settled after Resume: %v %v", changed, err)
	}
}

func TestSettleNotSent_StillClosesAReservedRowSendFailed(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-r", ceilingSession, "", 500)
	f.linkWake(t, "w1", "ut-r")
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET reserved_turn_id = 'rt-1' WHERE id = 'ut-r'`)
	f.env.svc.wakeFinder = &fakeFinder{}
	f.env.svc.settleNotSent(t.Context(), "ut-r", outcomeSendFailed)
	if got := f.row(t, "ut-r").Outcome.String; got != "send_failed" {
		t.Fatalf("outcome = %q", got)
	}
	if status, _ := f.wakeState(t, "w1"); status != "pending" {
		t.Fatalf("wake = %s", status)
	}
}

func TestStop_RacingRecoverUnboundTurnOneWinsWakesPending(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-u", ceilingSession, "", 500)
	f.linkWake(t, "w1", "ut-u")
	old := time.Now().UTC().Add(-time.Hour)
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET started_at = ? WHERE id = 'ut-u'`, old)
	f.env.svc.convReader = &fakeConvReader{states: map[string]string{ceilingSession: "WAITING_FOR_INPUT"}}
	f.pause(t)
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := f.env.store.getUnattendedTurn(t.Context(), "ut-u")
	if err != nil {
		t.Fatal(err)
	}
	f.env.svc.recoverUnboundTurn(t.Context(), turn)
	if got := f.row(t, "ut-u").Outcome.String; got != "stopped_by_pause" && got != "send_failed" {
		t.Fatalf("outcome = %q", got)
	}
	if status, _ := f.wakeState(t, "w1"); status != "pending" {
		t.Fatalf("wake = %s, want pending", status)
	}
}

func TestStop_CallsTheRegisteredDreamStopOnlyWhilePaused(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	var stopped []string
	f.env.svc.SetDreamStop(func(_ context.Context, id string) error {
		stopped = append(stopped, id)
		return nil
	})
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil || len(stopped) != 0 {
		t.Fatalf("stopped while not paused: %v %v", stopped, err)
	}
	f.pause(t)
	if err := f.env.svc.Stop(t.Context(), f.env.c.ID); err != nil || len(stopped) != 1 || stopped[0] != f.env.c.ID {
		t.Fatalf("dream stop = %v err=%v", stopped, err)
	}
}

func TestStopPausedCoordinators_RetriesForAnAutonomyOffCoordinator(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = 0 WHERE id = ?`, f.env.c.ID)
	f.pause(t)
	f.canceller.fn = func(context.Context, string, string) error { return errors.New("down") }
	var skipped []string
	skip := func(what string, _ error) { skipped = append(skipped, what) }
	f.env.svc.stopPausedCoordinators(t.Context(), skip)
	if f.row(t, ceilingTurnRow).Outcome.Valid {
		t.Fatal("settled despite a failed cancel")
	}
	f.canceller.fn = nil
	f.env.svc.stopPausedCoordinators(t.Context(), skip)
	if f.row(t, ceilingTurnRow).Outcome.String != "stopped_by_pause" || len(skipped) != 0 {
		t.Fatalf("row = %+v skipped=%v", f.row(t, ceilingTurnRow), skipped)
	}
	if !f.env.svc.knownPaused.Has(f.env.c.ID) {
		t.Fatal("known-paused set was not refreshed by the pass")
	}
}

func TestStopPausedCoordinators_FailedQuerySkipsThePass(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	mustExec(t, f.env.store, `ALTER TABLE coordinators RENAME TO coordinators_gone`)
	var skipped []string
	f.env.svc.stopPausedCoordinators(t.Context(), func(what string, _ error) { skipped = append(skipped, what) })
	if len(skipped) != 1 || skipped[0] != "paused coordinators" || f.canceller.callCount() != 0 {
		t.Fatalf("skipped=%v cancels=%d", skipped, f.canceller.callCount())
	}
}
