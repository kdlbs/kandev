package coordinator

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestBackstop_RetriesARecordedDenialWhoseFirstResolveFailed(t *testing.T) {
	for _, autonomy := range []int{1, 0} {
		f := newDeliverFixture(t)
		resolver := &fakePermissionResolver{}
		f.svc.SetUnattendedPermissionResolver(resolver)
		f.withFinder(&fakeFinder{})
		f.reader.turns = map[string]*TurnInfo{"st": {}}
		f.active.turns[ceilingSession] = "st"
		f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st", message: "m"})
		mustExec(t, f.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, autonomy, f.c.ID)
		if _, matched, err := f.store.RecordUnattendedDenial(context.Background(), ceilingConvTask, ceilingSession, "p1", "st"); err != nil || !matched {
			t.Fatalf("record denial: matched=%v err=%v", matched, err)
		}
		f.duties(t)
		if len(resolver.calls) != 1 || resolver.calls[0] != (resolverCall{ceilingConvTask, ceilingSession, "p1", "ut"}) {
			t.Fatalf("autonomy=%d: resolve calls = %+v, want one retry of p1", autonomy, resolver.calls)
		}
	}
}

func TestStartupPass_UsesTheProcessT0AsTheBoundary(t *testing.T) {
	processT0 := time.Now().UTC().Add(-time.Hour)
	for _, tc := range []struct {
		name    string
		started time.Time
		want    string
	}{
		{"in flight in this process", processT0.Add(time.Minute), ""},
		{"left by a previous process", processT0.Add(-time.Minute), outcomeInterrupted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			f.withFinder(&fakeFinder{})
			f.openTurnRow(t, openTurn{id: "ut", started: tc.started})
			if err := f.svc.RecoverUnattendedStartup(context.Background(), processT0); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				f.assertOpen(t, "ut")
				return
			}
			f.assertSettled(t, "ut", tc.want)
		})
	}
}

func TestDeliver_AStoredMessageWithAnAmbiguousErrorIsMarkedOrphanWhenTheBackstopSettles(t *testing.T) {
	f := newDeliverFixture(t)
	finder := &fakeFinder{}
	f.withFinder(finder)
	f.wake(t, "t1", "one", time.Now().UTC())
	f.sender.err = context.DeadlineExceeded
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	tr := f.turns(t)[0]
	if tr.MessageID.String != "msg-1" {
		t.Fatalf("message_id = %q, want the stored message recorded", tr.MessageID.String)
	}
	mustExec(t, f.store, `UPDATE coordinator_unattended_turns SET started_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Hour), tr.ID)
	f.reader.states[ceilingSession] = string(taskmodels.TaskSessionStateWaitingForInput)
	f.duties(t)
	f.assertSettled(t, tr.ID, outcomeSendFailed)
	if len(finder.marked) != 1 || finder.marked[0] != [2]string{ceilingSession, "msg-1"} {
		t.Fatalf("marked = %v, want the stored message marked orphaned", finder.marked)
	}
}
