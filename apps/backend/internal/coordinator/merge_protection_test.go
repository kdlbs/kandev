package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

type fakeProtection struct {
	p    mergestate.Protection
	err  error
	seen []string
}

func (f *fakeProtection) ReadProtection(_ context.Context, _, repo, branch string) (mergestate.Protection, error) {
	f.seen = append(f.seen, repo+"@"+branch)
	return f.p, f.err
}

func TestObserveProtectionRecordsAndFailureIsUnknown(t *testing.T) {
	e, _ := newObserveEnv(t)
	fp := &fakeProtection{p: mergestate.Protection{Pipeline: mergestate.Enforced, Approvals: mergestate.NotEnforced, Threads: mergestate.Enforced}}
	e.svc.SetProtectionReader(fp)
	obs, err := e.svc.ObserveProtection(context.Background(), "ws-1", "r1", "main")
	if err != nil || obs.Pipeline != mergestate.Enforced || obs.Approvals != mergestate.NotEnforced {
		t.Fatalf("obs = %+v err = %v", obs, err)
	}
	fp.err = errors.New("403")
	obs, err = e.svc.ObserveProtection(context.Background(), "ws-1", "r1", "main")
	if err != nil || obs.Pipeline != mergestate.UnknownEnf || obs.Approvals != mergestate.UnknownEnf || obs.Threads != mergestate.UnknownEnf {
		t.Fatalf("a failed read must record unknown: %+v err=%v", obs, err)
	}
	stored, _ := e.store.listProtection(context.Background(), "ws-1")
	if len(stored) != 1 || stored[0].Pipeline != mergestate.UnknownEnf {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestObserveProtectionWithoutReaderIsUnknown(t *testing.T) {
	e, _ := newObserveEnv(t)
	obs, err := e.svc.ObserveProtection(context.Background(), "ws-1", "r1", "")
	if err != nil || obs.Threads != mergestate.UnknownEnf {
		t.Fatalf("obs = %+v err = %v", obs, err)
	}
}

func TestRefreshProtectionCoversNonDeniedRepositoriesAndBranches(t *testing.T) {
	e, r := newObserveEnv(t)
	r.snap = mergestate.Snapshot{Key: testKey, State: mergestate.StateOpen, TargetBranch: "release", UnresolvedThreads: -1, Behind: -1}
	fp := &fakeProtection{}
	e.svc.SetProtectionReader(fp)
	mustExec(t, e.store, `INSERT INTO coordinator_merge_repos (coordinator_id, repository_id, policy, selected_by, automatic_since_revision, updated_at)
		VALUES (?, 'r1', 'requires_approval', '', 0, CURRENT_TIMESTAMP), (?, 'r2', 'requires_approval', '', 0, CURRENT_TIMESTAMP), (?, 'r3', 'denied', '', 0, CURRENT_TIMESTAMP)`,
		e.fix.c.ID, e.fix.c.ID, e.fix.c.ID)
	if _, err := e.svc.RefreshProtection(context.Background(), "ws-1", e.fix.c.ID); err != nil {
		t.Fatal(err)
	}
	want := []string{"r1@release", "r2@"}
	if len(fp.seen) != 2 || fp.seen[0] != want[0] || fp.seen[1] != want[1] {
		t.Fatalf("reads = %v want %v", fp.seen, want)
	}
}

func TestLeavingDeniedRecordsProtection(t *testing.T) {
	store, c, svc := mergeEnv(t)
	fp := &fakeProtection{p: mergestate.Protection{Pipeline: mergestate.Enforced, Approvals: mergestate.Enforced, Threads: mergestate.NotEnforced}}
	svc.SetProtectionReader(fp)
	rev := storedRevision(t, store, c.ID)
	if _, err := saveMerge(svc, c, rev, `{"repository_id":"r1","policy":"requires_approval"}`); err != nil {
		t.Fatal(err)
	}
	if len(fp.seen) != 1 || fp.seen[0] != "r1@" {
		t.Fatalf("reads = %v", fp.seen)
	}
	got, _ := svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if len(got.Merge.Protection) != 1 || got.Merge.Protection[0].Approvals != mergestate.Enforced {
		t.Fatalf("protection = %+v", got.Merge.Protection)
	}
	// A repository staying non-denied, or going back to denied, reads nothing.
	if _, err := saveMerge(svc, c, rev+1, `{"repository_id":"r1","policy":"automatic"}`); err != nil {
		t.Fatal(err)
	}
	if len(fp.seen) != 1 {
		t.Fatalf("only the first leave of denied reads: %v", fp.seen)
	}
}
