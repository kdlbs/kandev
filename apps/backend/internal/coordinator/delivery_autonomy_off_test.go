package coordinator

import (
	"fmt"
	"testing"
	"time"
)

func TestBackstopDuties_RunWithAutonomyOffAndEveryEventSuppressed(t *testing.T) {
	f := newDeliverFixture(t)
	mustExec(t, f.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.c.ID)
	finder := &fakeFinder{msgs: map[string]*WakeMessage{"ut": {ID: "m-1", TurnID: "rt"}}}
	f.withFinder(finder)
	f.reader.turns = map[string]*TurnInfo{"st": {Completed: true}}
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))

	f.duties(t)

	f.assertSettled(t, "ut", outcomeCompleted)
	if _, _, _, msg := f.turnRow(t, "ut"); msg.String != "m-1" {
		t.Fatalf("message_id = %q, want m-1", msg.String)
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := f.sender.count(); got != 0 {
		t.Fatalf("sent %d prompts with autonomy off, want 0", got)
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != "pending" {
		t.Fatalf("wake status = %q, want pending", status)
	}
}

func TestDeliver_SupersedesMoreThanFiftyEndedWakesInOnePass(t *testing.T) {
	f := newDeliverFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 60; i++ {
		f.wake(t, fmt.Sprintf("t%02d", i), "ended", base.Add(time.Duration(i)*time.Second))
	}
	f.sources.mu.Lock()
	f.sources.question = map[string]string{}
	f.sources.mu.Unlock()
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := f.sender.count(); got != 0 {
		t.Fatalf("sent %d prompts, want 0 when no wake holds", got)
	}
	for i := 0; i < 60; i++ {
		if status, _ := f.wakeStatus(t, fmt.Sprintf("w-t%02d", i)); status != wakeStatusSuperseded {
			t.Fatalf("wake %d status = %q, want superseded", i, status)
		}
	}
}
