package coordinator

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// noTurnStartWakeBackstop proves the recovery duties (backstop turn duties and
// the startup pass) never send a prompt, even with a pending wake and an open,
// unbound turn to recover.
func noTurnStartWakeBackstop(t *testing.T) {
	f := newDeliverFixture(t)
	f.withFinder(&fakeFinder{})
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.openTurnRow(t, openTurn{id: "ut", started: time.Now().UTC().Add(-time.Hour)})
	f.duties(t)
	if err := f.svc.RecoverUnattendedStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.sender.count(); got != 0 {
		t.Fatalf("recovery sent %d prompts, want 0", got)
	}
}

// noTurnStartWakeAdmissionHeld proves delivery sends nothing while admission
// holds.
func noTurnStartWakeAdmissionHeld(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.reader.states[ceilingSession] = string(taskmodels.TaskSessionStateRunning)
	f.reader.primary.State = string(taskmodels.TaskSessionStateRunning)
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := f.sender.count(); got != 0 {
		t.Fatalf("a held admission sent %d prompts, want 0", got)
	}
}

// noTurnStartWakeDelivery is the one allowed non-manager turn start: delivery
// with admission passed sends exactly one unattended prompt to the
// coordinator's conversation session.
func noTurnStartWakeDelivery(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "one", time.Now().UTC().Add(-time.Hour))
	f.wake(t, "t2", "two", time.Now().UTC().Add(-time.Hour))
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := f.sender.count(); got != 1 {
		t.Fatalf("delivery sent %d prompts, want exactly 1 (one open turn at a time)", got)
	}
	if call := f.sender.calls[0]; call.SessionID != ceilingSession || call.TaskID != ceilingConvTask {
		t.Fatalf("prompt went to %s/%s, want the conversation session", call.TaskID, call.SessionID)
	}
}
