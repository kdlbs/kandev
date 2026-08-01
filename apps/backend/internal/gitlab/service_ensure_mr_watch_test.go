package gitlab

import (
	"context"
	"testing"
)

func TestEnsureMRWatch_CreatesWhenMissing(t *testing.T) {
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	ctx := context.Background()

	w, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 5, "feat/a")
	if err != nil {
		t.Fatalf("ensure watch: %v", err)
	}
	if w == nil || w.ID == "" || w.MRIID != 5 || w.Branch != "feat/a" {
		t.Fatalf("unexpected watch: %+v", w)
	}

	got, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil || got == nil || got.ID != w.ID {
		t.Fatalf("watch not persisted: %+v err=%v", got, err)
	}
}

func TestEnsureMRWatch_IdempotentReturnsExistingRow(t *testing.T) {
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	ctx := context.Background()

	first, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 5, "feat/a")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 5, "feat/a")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same watch row, got %s and %s", first.ID, second.ID)
	}

	all, err := store.ListMRWatchesBySession(ctx, "sess-1")
	if err != nil || len(all) != 1 {
		t.Fatalf("expected exactly 1 watch row, got %d err=%v", len(all), err)
	}
}

func TestEnsureMRWatch_UpdatesIIDWhenPreviouslyUnknown(t *testing.T) {
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	ctx := context.Background()

	first, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 0, "feat/a")
	if err != nil {
		t.Fatalf("first ensure (unknown iid): %v", err)
	}
	if first.MRIID != 0 {
		t.Fatalf("expected iid 0 on first ensure, got %d", first.MRIID)
	}

	second, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 9, "feat/a")
	if err != nil {
		t.Fatalf("second ensure (now known iid): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same watch row updated in place, got %s and %s", first.ID, second.ID)
	}
	if second.MRIID != 9 {
		t.Fatalf("expected iid backfilled to 9, got %d", second.MRIID)
	}

	got, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil || got == nil || got.MRIID != 9 {
		t.Fatalf("persisted iid not updated: %+v err=%v", got, err)
	}
}

// TestEnsureMRWatch_ReplacesIIDWhenBranchRelinkedToNewMR covers a watch that
// already knows a positive iid, not just the "previously unknown" case: the
// original MR closed and a new one opened on the same branch. Without
// replacing a known-but-stale iid, the watch would keep polling the closed
// MR forever.
func TestEnsureMRWatch_ReplacesIIDWhenBranchRelinkedToNewMR(t *testing.T) {
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	ctx := context.Background()

	first, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 5, "feat/a")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if first.MRIID != 5 {
		t.Fatalf("expected iid 5, got %d", first.MRIID)
	}

	second, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 11, "feat/a")
	if err != nil {
		t.Fatalf("second ensure (replacement MR): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same watch row updated in place, got %s and %s", first.ID, second.ID)
	}
	if second.MRIID != 11 {
		t.Fatalf("expected iid replaced with 11, got %d", second.MRIID)
	}

	got, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil || got == nil || got.MRIID != 11 {
		t.Fatalf("persisted iid not replaced: %+v err=%v", got, err)
	}
}

func TestEnsureMRWatch_MultiBranchCoexist(t *testing.T) {
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	ctx := context.Background()

	if _, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 1, "feat/a"); err != nil {
		t.Fatalf("ensure branch a: %v", err)
	}
	if _, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 2, "feat/b"); err != nil {
		t.Fatalf("ensure branch b: %v", err)
	}

	all, err := store.ListMRWatchesBySession(ctx, "sess-1")
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 coexisting branch watches, got %d err=%v", len(all), err)
	}
}
