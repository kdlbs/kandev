package worktree

import (
	"context"
	"testing"
	"time"
)

// helpers ---------------------------------------------------------------------

func newMultiRepoTestManager(t *testing.T) (*Manager, *mockStore) {
	t.Helper()
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr, store
}

func seedWorktree(store *mockStore, sessionID, repoID, id string) *Worktree {
	wt := &Worktree{
		ID:           id,
		SessionID:    sessionID,
		RepositoryID: repoID,
		TaskID:       "task-x",
		Path:         "/tmp/wt/" + id,
		Branch:       "feat/" + id,
		Status:       StatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	store.worktrees[id] = wt
	return wt
}

// tests -----------------------------------------------------------------------

func TestManager_GetAllBySessionID_ReturnsAllRepos(t *testing.T) {
	mgr, store := newMultiRepoTestManager(t)
	seedWorktree(store, "sess-1", "repo-front", "wt-1")
	seedWorktree(store, "sess-1", "repo-back", "wt-2")
	seedWorktree(store, "sess-2", "repo-front", "wt-3") // unrelated session

	got, err := mgr.GetAllBySessionID(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetAllBySessionID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 worktrees for sess-1, got %d", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["wt-1"] || !ids["wt-2"] {
		t.Errorf("unexpected ids returned: %v", ids)
	}
}

func TestManager_GetBySessionAndRepo_PicksTheRightOne(t *testing.T) {
	mgr, store := newMultiRepoTestManager(t)
	seedWorktree(store, "sess-1", "repo-front", "wt-1")
	seedWorktree(store, "sess-1", "repo-back", "wt-2")

	got, err := mgr.GetBySessionAndRepo(context.Background(), "sess-1", "repo-back")
	if err != nil {
		t.Fatalf("GetBySessionAndRepo: %v", err)
	}
	if got.ID != "wt-2" {
		t.Errorf("expected wt-2 for repo-back, got %s", got.ID)
	}
}

func TestManager_GetBySessionAndRepo_NotFoundForUnknownRepo(t *testing.T) {
	mgr, store := newMultiRepoTestManager(t)
	seedWorktree(store, "sess-1", "repo-front", "wt-1")

	_, err := mgr.GetBySessionAndRepo(context.Background(), "sess-1", "nope")
	if err == nil {
		t.Fatal("expected ErrWorktreeNotFound for unknown repo")
	}
}

func TestManager_GetBySessionID_ReturnsAnyMatch_SingleRepoCompat(t *testing.T) {
	mgr, store := newMultiRepoTestManager(t)
	seedWorktree(store, "sess-1", "repo-only", "wt-1")

	got, err := mgr.GetBySessionID(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if got.ID != "wt-1" {
		t.Errorf("expected wt-1, got %s", got.ID)
	}
}

func TestManager_CacheKey_SeparatesReposPerSession(t *testing.T) {
	// Two cache entries for the same session must not collide. We simulate by
	// directly writing to the cache the same way persistAndCacheWorktree does.
	mgr, _ := newMultiRepoTestManager(t)
	wtA := &Worktree{ID: "wt-A", SessionID: "s", RepositoryID: "rA", Path: "/a", Status: StatusActive}
	wtB := &Worktree{ID: "wt-B", SessionID: "s", RepositoryID: "rB", Path: "/b", Status: StatusActive}
	mgr.mu.Lock()
	mgr.worktrees[cacheKey("s", "rA")] = wtA
	mgr.worktrees[cacheKey("s", "rB")] = wtB
	mgr.mu.Unlock()

	gotA, err := mgr.GetBySessionAndRepo(context.Background(), "s", "rA")
	if err != nil || gotA.ID != "wt-A" {
		t.Errorf("expected wt-A from cache; got %v err=%v", gotA, err)
	}
	gotB, err := mgr.GetBySessionAndRepo(context.Background(), "s", "rB")
	if err != nil || gotB.ID != "wt-B" {
		t.Errorf("expected wt-B from cache; got %v err=%v", gotB, err)
	}
}
