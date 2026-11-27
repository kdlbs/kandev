package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// raceEnv runs the Apply, Discard and PATCH entry points against one store so
// the same interleavings can be checked on SQLite and PostgreSQL.
type raceEnv struct {
	store *Store
	svc   *Service
	c     *Coordinator
}

func newRaceEnv(t *testing.T, store *Store) *raceEnv {
	t.Helper()
	c := newTestCoordinator(t, store, "ws-1")
	svc := NewService(store, newValidatorForTest(
		map[string]*settingsmodels.AgentProfile{"a": {ID: "a", WorkspaceID: "ws-1"}},
		map[string]*taskmodels.ExecutorProfile{"e": {ID: "e"}}), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	svc.phase3 = true
	return &raceEnv{store: store, svc: svc, c: c}
}

// seed stores an approved improvement with a pending change over the current
// context and returns the change id.
func (e *raceEnv) seed(t *testing.T, base, next string) string {
	t.Helper()
	mustExec(t, e.store, `UPDATE coordinators SET context = ? WHERE id = ?`, base, e.c.ID)
	p := &Proposal{CoordinatorID: e.c.ID, WorkspaceID: e.c.WorkspaceID, Kind: ProposalKindImprovement, RawSpec: `{}`}
	if err := e.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	mustExec(t, e.store, `UPDATE coordinator_proposals SET status = 'approved' WHERE id = ?`, p.ID)
	if err := e.store.InsertPendingChange(context.Background(), e.c.ID, p.ID, base, next); err != nil {
		t.Fatal(err)
	}
	changes, err := e.store.ListPendingChanges(context.Background(), e.c.ID)
	if err != nil || len(changes) == 0 {
		t.Fatalf("list: %v %v", changes, err)
	}
	return changes[len(changes)-1].ID
}

func (e *raceEnv) contextValue(t *testing.T) string {
	t.Helper()
	c, err := e.store.GetCoordinatorByID(context.Background(), e.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Context
}

func (e *raceEnv) changeStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := e.store.db.Get(&status, e.store.db.Rebind(`SELECT status FROM coordinator_pending_changes WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
	return status
}

func raceTwo(a, b func() error) (error, error) {
	var wg sync.WaitGroup
	var errA, errB error
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; errA = a() }()
	go func() { defer wg.Done(); <-start; errB = b() }()
	close(start)
	wg.Wait()
	return errA, errB
}

const raceRounds = 20

func (e *raceEnv) apply(id string) error {
	_, err := e.svc.ApplyPendingChange(context.Background(), "ws-1", e.c.ID, id)
	return err
}

func (e *raceEnv) assertApplyTwice(t *testing.T) {
	t.Helper()
	for range raceRounds {
		id := e.seed(t, "base", "next")
		a, b := raceTwo(func() error { return e.apply(id) }, func() error { return e.apply(id) })
		if (a == nil) == (b == nil) {
			t.Fatalf("want exactly one winner, got %v and %v", a, b)
		}
		loser := errors.Join(a, b)
		var conflict *ChangeConflictError
		if !errors.As(loser, &conflict) {
			t.Fatalf("loser err = %v", loser)
		}
		if e.contextValue(t) != "next" || e.changeStatus(t, id) != ChangeStatusApplied {
			t.Fatalf("context=%q status=%q", e.contextValue(t), e.changeStatus(t, id))
		}
	}
}

func (e *raceEnv) assertApplyVersusDiscard(t *testing.T) {
	t.Helper()
	for range raceRounds {
		id := e.seed(t, "base", "next")
		a, b := raceTwo(func() error { return e.apply(id) }, func() error {
			_, err := e.svc.DiscardPendingChange(context.Background(), "ws-1", e.c.ID, id)
			return err
		})
		if (a == nil) == (b == nil) {
			t.Fatalf("want exactly one winner, got %v and %v", a, b)
		}
		switch e.changeStatus(t, id) {
		case ChangeStatusApplied:
			if a != nil || e.contextValue(t) != "next" {
				t.Fatalf("applied but apply err=%v context=%q", a, e.contextValue(t))
			}
		case ChangeStatusDiscarded:
			if b != nil || e.contextValue(t) != "base" {
				t.Fatalf("discarded but discard err=%v context=%q", b, e.contextValue(t))
			}
		default:
			t.Fatalf("change left pending")
		}
	}
}

func (e *raceEnv) assertApplyVersusPatch(t *testing.T) {
	t.Helper()
	for range raceRounds {
		id := e.seed(t, "base", "next")
		var applyErr error
		_, patchErr := func() (struct{}, error) {
			a, b := raceTwo(func() error { return e.apply(id) }, func() error {
				_, err := e.svc.PatchCoordinator(context.Background(), "ws-1", e.c.ID,
					PatchCoordinatorRequest{"context": []byte(`"patched"`)})
				return err
			})
			applyErr = a
			return struct{}{}, b
		}()
		if patchErr != nil {
			t.Fatalf("patch: %v", patchErr)
		}
		status := e.changeStatus(t, id)
		if applyErr == nil {
			if status != ChangeStatusApplied {
				t.Fatalf("apply ok but status %q", status)
			}
			continue
		}
		var conflict *ChangeConflictError
		if !errors.As(applyErr, &conflict) || conflict.Reason != ChangeReasonContextChanged {
			t.Fatalf("apply err = %v", applyErr)
		}
		if status != ChangeStatusPending || e.contextValue(t) != "patched" {
			t.Fatalf("lost patch: status=%q context=%q", status, e.contextValue(t))
		}
	}
}

func TestImprovementApplyRaces(t *testing.T) {
	e := newRaceEnv(t, newTestStore(t))
	e.assertApplyTwice(t)
	e.assertApplyVersusDiscard(t)
	e.assertApplyVersusPatch(t)
}
