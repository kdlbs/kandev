package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
)

func TestWakeDeliveredCounterRequiresActualPromptAcceptance(t *testing.T) {
	f := newDeliverFixture(t)
	now := time.Now().UTC()
	f.store.now = func() time.Time { return now }
	f.wake(t, "t1", "waiting", now)
	f.sender.err = errors.Join(orchestrator.ErrWakePromptNotDispatched, errors.New("busy"))
	before := wakeDeliveredTotal.Value()
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := wakeDeliveredTotal.Value(); got != before {
		t.Fatalf("refused wake counted delivered: %d -> %d", before, got)
	}
	now = now.Add(unattendedCooldown)
	f.sender.err, f.sender.accept = nil, "accepted-turn"
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if got := wakeDeliveredTotal.Value(); got != before+1 {
		t.Fatalf("accepted wake count=%d, want %d", got, before+1)
	}
	if f.sender.count() != 2 {
		t.Fatalf("acceptance path did not run after cooldown: %d sends", f.sender.count())
	}
	f.sender.calls[1].OnAccepted("accepted-turn")
	if got := wakeDeliveredTotal.Value(); got != before+1 {
		t.Fatalf("duplicate acceptance counted again: %d, want %d", got, before+1)
	}
}

func TestDeliveryRejectsProjectRepairDuringEpisodeRecheck(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "waiting", time.Now().UTC())
	projects := newFakeProjects()
	projects.addRepo("repo-a", "A")
	projects.taskRepos["t1"] = []string{"repo-a"}
	f.svc.SetProjectReader(projects)
	projectScope(t, f.store, f.c.ID, false, repoEntry("repo-a"))
	f.hooks.onRead = func() {
		if err := f.svc.ProjectDeleted(context.Background(), projectKindRepository, "repo-a"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatal("wake dispatched with a project scope repaired after recheck")
	}
	if status, _ := f.wakeStatus(t, "w-t1"); status != wakeStatusPending {
		t.Fatalf("unclaimed wake became %q", status)
	}
}

func TestDeliveryRejectsTaskMovedToAnotherWorkspaceDuringRecheck(t *testing.T) {
	f := newDeliverFixture(t)
	f.wake(t, "t1", "waiting", time.Now().UTC())
	f.hooks.onRead = func() { mustExec(t, f.store, `UPDATE tasks SET workspace_id = 'foreign' WHERE id = 't1'`) }
	if err := f.deliver(); err != nil {
		t.Fatal(err)
	}
	if f.sender.count() != 0 {
		t.Fatal("foreign-workspace task admitted by stale ownership snapshot")
	}
}
