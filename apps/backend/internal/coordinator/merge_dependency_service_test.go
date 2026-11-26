package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const (
	depKeyA = "github|github.com|r1|1"
	depKeyB = "github|github.com|r1|2"
)

func depEnv(t *testing.T) (*wakeEnv, *fakeChangeReader) {
	t.Helper()
	e, r := newObserveEnv(t)
	e.fix.addTask(t, "ta", "wf-a")
	e.fix.addTask(t, "tb", "wf-a")
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{depKeyA: {"ta"}, depKeyB: {"tb"}, testChangeKey: {"tm"}}})
	return e, r
}

func saveDeps(e *wakeEnv, card string, rev int, refs ...DepRef) (DepSet, error) {
	return e.svc.SaveDependencies(managerCtx("u1"), "ws-1", e.fix.c.ID, card, true, rev, refs)
}

func TestDependencyRevisionRequired(t *testing.T) {
	e, _ := depEnv(t)
	_, err := e.svc.SaveDependencies(managerCtx("u1"), "ws-1", e.fix.c.ID, "tm", false, 0, []DepRef{{Task: "ta"}})
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != codeRevisionRequired {
		t.Fatalf("err = %v", err)
	}
}

func TestDependencyRevisionConflict(t *testing.T) {
	e, _ := depEnv(t)
	if _, err := saveDeps(e, "tm", 0, DepRef{Task: "ta"}); err != nil {
		t.Fatal(err)
	}
	_, err := saveDeps(e, "tm", 0, DepRef{Task: "tb"})
	var c *MergeConflictError
	if !errors.As(err, &c) || c.Code != codeRevisionConflict {
		t.Fatalf("stale save: %v", err)
	}
	got, _ := e.svc.GetDependencies(managerCtx("u1"), "ws-1", e.fix.c.ID, "tm")
	if got.Revision != 1 || len(got.Refs) != 1 || got.Refs[0].Task != "ta" {
		t.Fatalf("set = %+v", got)
	}
}

func TestDependencyAuditAndUnchangedSave(t *testing.T) {
	e, _ := depEnv(t)
	next, err := saveDeps(e, "tm", 0, DepRef{Task: "ta"}, DepRef{Change: depKeyB})
	if err != nil || next.Revision != 1 {
		t.Fatalf("next = %+v err = %v", next, err)
	}
	same, err := saveDeps(e, "tm", 1, DepRef{Change: depKeyB}, DepRef{Task: "ta"})
	if err != nil || same.Revision != 1 {
		t.Fatalf("an unchanged set must not bump the revision: %+v %v", same, err)
	}
	var n int
	_ = e.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_merge_audit WHERE kind = 'dependencies_changed' AND subject = 'tm'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestDependencySaveRefusalsStoreNothing(t *testing.T) {
	e, _ := depEnv(t)
	for _, refs := range [][]DepRef{{{Task: "tm"}}, {{Task: "ta"}, {Task: "ta"}}, {{Task: "ghost"}}, {{}}} {
		if _, err := saveDeps(e, "tm", 0, refs...); err == nil {
			t.Fatalf("refs %+v accepted", refs)
		}
	}
	got, _ := e.svc.GetDependencies(managerCtx("u1"), "ws-1", e.fix.c.ID, "tm")
	if got.Revision != 0 || len(got.Refs) != 0 {
		t.Fatalf("a refused save stored %+v", got)
	}
}

func TestDependencySaveRequiresManagerAndFlag(t *testing.T) {
	e, _ := depEnv(t)
	e.svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	if _, err := saveDeps(e, "tm", 0, DepRef{Task: "ta"}); !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	e.svc.authz = &fakeWorkspaceAuthorizer{}
	e.svc.merge = false
	if _, err := saveDeps(e, "tm", 0, DepRef{Task: "ta"}); err == nil {
		t.Fatal("saved with the flag off")
	}
}

func TestDependencyLockAtRecheck(t *testing.T) {
	e, r := depEnv(t)
	if _, err := saveDeps(e, "tm", 0, DepRef{Task: "ta"}); err != nil {
		t.Fatal(err)
	}
	r.snap = mergestate.Snapshot{Key: testKey, State: mergestate.StateOpen}
	tri, rev, err := e.svc.predecessorState(context.Background(), "ws-1", e.fix.c.ID, "tm")
	if err != nil || tri != mergestate.Unmet || rev != 1 {
		t.Fatalf("unmerged predecessor: tri=%v rev=%d err=%v", tri, rev, err)
	}
	r.snap = mergestate.Snapshot{Key: testKey, State: mergestate.StateMerged}
	tri, _, _ = e.svc.predecessorState(context.Background(), "ws-1", e.fix.c.ID, "tm")
	if tri != mergestate.Met {
		t.Fatalf("merged predecessor: tri=%v", tri)
	}
	r.err = errors.New("down")
	tri, _, _ = e.svc.predecessorState(context.Background(), "ws-1", e.fix.c.ID, "tm")
	if tri != mergestate.Unavailable {
		t.Fatalf("failed read must be unavailable, got %v", tri)
	}
	// No set is no predecessors.
	tri, rev, _ = e.svc.predecessorState(context.Background(), "ws-1", e.fix.c.ID, "tb")
	if tri != mergestate.Met || rev != 0 {
		t.Fatalf("empty set: tri=%v rev=%d", tri, rev)
	}
}
