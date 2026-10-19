package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func setFixtureSessionState(t *testing.T, f *cancelTurnFixture, state models.TaskSessionState) {
	t.Helper()
	if err := f.repo.UpdateTaskSessionState(context.Background(), "s1", state, ""); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteUnattendedOrphanTurn_CompletesMatchingTurnAndClearsCache(t *testing.T) {
	f := newCancelTurnFixture(t)
	setFixtureSessionState(t, f, models.TaskSessionStateWaitingForInput)
	f.svc.activeTurns.Store("s1", "turn-1")

	if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if id := f.activeTurnID(t); id != "" {
		t.Fatalf("turn %q is still active", id)
	}
	if _, ok := f.svc.activeTurns.Load("s1"); ok {
		t.Fatal("activeTurns entry survived the completion")
	}
}

func TestCompleteUnattendedOrphanTurn_NoActiveTurnStillClearsStaleCacheEntry(t *testing.T) {
	f := newCancelTurnFixture(t)
	setFixtureSessionState(t, f, models.TaskSessionStateWaitingForInput)
	if err := f.repo.CompleteTurn(context.Background(), "turn-1"); err != nil {
		t.Fatal(err)
	}
	f.svc.activeTurns.Store("s1", "turn-1")

	if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatalf("no active turn must return nil: %v", err)
	}
	if _, ok := f.svc.activeTurns.Load("s1"); ok {
		t.Fatal("stale activeTurns entry for the completed turn survived the nil path")
	}
}

func TestCompleteUnattendedOrphanTurn_NoActiveTurnKeepsSuccessorCacheEntry(t *testing.T) {
	f := newCancelTurnFixture(t)
	setFixtureSessionState(t, f, models.TaskSessionStateWaitingForInput)
	if err := f.repo.CompleteTurn(context.Background(), "turn-1"); err != nil {
		t.Fatal(err)
	}
	f.svc.activeTurns.Store("s1", "turn-2")

	if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.svc.activeTurns.Load("s1"); !ok || v != "turn-2" {
		t.Fatalf("successor entry = %v %v, want turn-2 kept", v, ok)
	}
}

func TestCompleteUnattendedOrphanTurn_BusySessionChangesNothing(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateRunning, models.TaskSessionStateStarting} {
		f := newCancelTurnFixture(t)
		setFixtureSessionState(t, f, state)
		f.svc.activeTurns.Store("s1", "turn-1")

		err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-1")
		if !errors.Is(err, ErrOrphanTurnSessionBusy) {
			t.Fatalf("state %s: err = %v, want ErrOrphanTurnSessionBusy", state, err)
		}
		if id := f.activeTurnID(t); id != "turn-1" {
			t.Fatalf("state %s: active turn = %q, want turn-1 untouched", state, id)
		}
		if _, ok := f.svc.activeTurns.Load("s1"); !ok {
			t.Fatalf("state %s: activeTurns entry was cleared", state)
		}
	}
}

func TestCompleteUnattendedOrphanTurn_SupersededTurnIsAnError(t *testing.T) {
	f := newCancelTurnFixture(t)
	setFixtureSessionState(t, f, models.TaskSessionStateWaitingForInput)

	if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-other"); err == nil {
		t.Fatal("an active turn that is not the orphan must be an error")
	}
	if id := f.activeTurnID(t); id != "turn-1" {
		t.Fatalf("active turn = %q, want turn-1 untouched", id)
	}
}

func TestCompleteUnattendedOrphanTurn_MissingSessionIsNotBusy(t *testing.T) {
	f := newCancelTurnFixture(t)
	if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "no-such-session", "turn-1"); err != nil {
		t.Fatalf("a missing session is not busy and has no active turn: %v", err)
	}
}
