package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/pause"
)

func failingGate(s *Service, effective bool, known ...string) {
	set := pause.NewKnownSet()
	for _, id := range known {
		set.Set(id, true)
	}
	s.knownPaused = set
	s.gate = pause.NewGate(func(context.Context, string) (bool, error) {
		return false, errors.New("state unreadable")
	}, func() bool { return effective }, set, s.logger.Zap())
}

func (f *deliverFixture) pauseNow(t *testing.T) {
	t.Helper()
	if _, err := f.store.SetPaused(context.Background(), f.c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
}

func (f *deliverFixture) assertUntouched(t *testing.T, wakeID string) {
	t.Helper()
	if status, turn := f.wakeStatus(t, wakeID); status != "pending" || turn.Valid {
		t.Fatalf("wake %s = %s/%v, want pending", wakeID, status, turn)
	}
	if len(f.turns(t)) != 0 || f.sender.count() != 0 {
		t.Fatalf("turns=%d sends=%d, want none", len(f.turns(t)), f.sender.count())
	}
}

func TestDeliver_PausedAdmitsSupersedesAndSendsNothing(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.wake(t, "t2", "two", time.Now().UTC().Add(-time.Hour))
	f.sources.mu.Lock()
	delete(f.sources.question, "t2")
	f.sources.mu.Unlock()
	f.pauseNow(t)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	f.assertUntouched(t, "w-t1")
	if status, _ := f.wakeStatus(t, "w-t2"); status != "pending" {
		t.Fatalf("a wake whose episode ended was superseded while paused: %s", status)
	}
}

func TestDeliver_ResumeDeliversThePendingWakes(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.sender.reserve, f.sender.accept = "rt-1", "st-1"
	f.pauseNow(t)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	f.assertUntouched(t, "w-t1")
	if _, err := f.store.SetPaused(t.Context(), f.c.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "delivered" || f.sender.count() != 1 {
		t.Fatalf("after Resume: status=%s sends=%d", status, f.sender.count())
	}
}

func TestDeliver_UnreadablePausedStateFailsClosedWhenEffective(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	failingGate(f.svc, true)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	f.assertUntouched(t, "w-t1")
}

func TestDeliver_FlagOffUnreadableStateOnlyHoldsAKnownPausedCoordinator(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.sender.reserve, f.sender.accept = "rt-1", "st-1"
	failingGate(f.svc, false, f.c.ID)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	f.assertUntouched(t, "w-t1")
	failingGate(f.svc, false)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 1 {
		t.Fatal("an unknown coordinator was held by an unreadable state with the flag off")
	}
}

func TestDeliver_PausedAfterTheGateStartsNoTurn(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.hooks.onRead = func() { f.pauseNow(t); f.hooks.onRead = nil }
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if len(f.turns(t)) != 0 || f.sender.count() != 0 {
		t.Fatalf("turns=%d sends=%d, want none after a pause between the gate and the start", len(f.turns(t)), f.sender.count())
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "pending" {
		t.Fatalf("wake = %s", status)
	}
}

func TestAutomaticApproval_PausedStaysPendingWithNoteAndIsNotCounted(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	if _, err := store.SetPaused(t.Context(), c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
	approved := expvarMapValue(automaticApprovalTotal, automaticApproved)
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusPending || res.Note != pausedNote {
		t.Fatalf("res = %+v", res)
	}
	if reload(t, store, c, p.ID).Status != ProposalStatusPending || len(tasks.createCalls) != 0 {
		t.Fatal("a paused coordinator approved or created")
	}
	if expvarMapValue(automaticApprovalTotal, automaticApproved) != approved {
		t.Fatal("the paused proposal advanced the counter")
	}
	if _, err := store.SetPaused(t.Context(), c.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if reload(t, store, c, p.ID).Status != ProposalStatusPending {
		t.Fatal("Resume approved a proposal automatically")
	}
}

func TestAutomaticApproval_UnreadablePausedStateNotesUnavailable(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	failingGate(svc, true)
	_, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
		t.Fatalf("res = %+v", res)
	}
}

func TestAutomaticApproval_FlagOffUnreadableStateProceedsForAnUnknownCoordinator(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	failingGate(svc, false)
	_, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v, want phase 3 behaviour", res)
	}
}

func TestOnAccepted_BindsAndStopsATurnAPauseFoundUnsent(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-u", ceilingSession, "", 500)
	f.pause(t)
	if _, err := f.env.store.markPauseRequested(t.Context(), "ut-u"); err != nil {
		t.Fatal(err)
	}
	f.env.svc.onAccepted(t.Context(), f.env.c.ID, ceilingSession, "ut-u", "st-9")
	f.env.svc.pauseRun.wg.Wait()
	if f.canceller.callCount() != 1 || f.canceller.calls[0] != [2]string{ceilingSession, "st-9"} {
		t.Fatalf("cancel calls = %v", f.canceller.calls)
	}
	if got := f.row(t, "ut-u").Outcome.String; got != "stopped_by_pause" {
		t.Fatalf("outcome = %q", got)
	}
}

func TestOnAccepted_UnmarkedTurnIsBoundAndLeftRunning(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	mustExec(t, f.env.store, `DELETE FROM coordinator_unattended_turns`)
	f.insertTurn(t, "ut-u", ceilingSession, "", 500)
	f.env.svc.onAccepted(t.Context(), f.env.c.ID, ceilingSession, "ut-u", "st-9")
	f.env.svc.pauseRun.wg.Wait()
	if f.canceller.callCount() != 0 || f.row(t, "ut-u").Outcome.Valid {
		t.Fatalf("an unpaused accepted turn was stopped: %d", f.canceller.callCount())
	}
}

func TestOnAccepted_LateSendOnAPausedCoordinatorIsCancelledOnce(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	late := pauseLateSendTotal.Value()
	failed := pauseLateSendFailedTotal.Value()
	f.env.svc.onAccepted(t.Context(), f.env.c.ID, ceilingSession, "ghost", "st-late")
	f.env.svc.pauseRun.wg.Wait()
	if f.canceller.callCount() != 1 || f.canceller.calls[0] != [2]string{ceilingSession, "st-late"} {
		t.Fatalf("cancel calls = %v", f.canceller.calls)
	}
	if pauseLateSendTotal.Value() != late+1 || pauseLateSendFailedTotal.Value() != failed {
		t.Fatalf("late=%d failed=%d", pauseLateSendTotal.Value()-late, pauseLateSendFailedTotal.Value()-failed)
	}
}

func TestOnAccepted_LateSendWhenNotPausedIsLeftAlone(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.env.svc.onAccepted(t.Context(), f.env.c.ID, ceilingSession, "ghost", "st-late")
	f.env.svc.pauseRun.wg.Wait()
	if f.canceller.callCount() != 0 {
		t.Fatal("cancelled a late send of a coordinator that is not paused")
	}
}

func TestOnAccepted_FailedLateCancelCountsOnceWithoutRetry(t *testing.T) {
	f := newCeilingFixture(t, 1000)
	f.pause(t)
	f.canceller.fn = func(context.Context, string, string) error { return errors.New("down") }
	failed := pauseLateSendFailedTotal.Value()
	f.env.svc.onAccepted(t.Context(), f.env.c.ID, ceilingSession, "ghost", "st-late")
	f.env.svc.pauseRun.wg.Wait()
	if f.canceller.callCount() != 1 || pauseLateSendFailedTotal.Value() != failed+1 {
		t.Fatalf("cancels=%d failed delta=%d", f.canceller.callCount(), pauseLateSendFailedTotal.Value()-failed)
	}
}
