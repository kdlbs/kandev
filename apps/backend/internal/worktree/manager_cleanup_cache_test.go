package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-RUNTIME-CLEANUP-001.58
func TestCleanupWorktrees_CacheAliases(t *testing.T) {
	for _, tc := range []struct {
		name     string
		noStore  bool
		preserve bool
	}{
		{name: "stored removal"},
		{name: "stored branch preservation", preserve: true},
		{name: "cache-only removal", noStore: true},
		{name: "cache-only branch preservation", noStore: true, preserve: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mgr, store, created, projected := cleanupCacheAliasFixture(t)
			survivors := createCleanupCacheSiblings(t, mgr, store, created)
			if tc.noStore {
				mgr.store = nil
				projected.SessionID = ""
			}
			var err error
			if tc.preserve {
				err = mgr.CleanupWorktreesPreservingBranches(ctx, []*Worktree{projected})
			} else {
				err = mgr.CleanupWorktrees(ctx, []*Worktree{projected})
			}
			require.NoError(t, err)
			require.NoDirExists(t, created.Path)
			assertNoCleanupRegistration(t, created.RepositoryPath, created.Path)
			assertCleanupCacheMiss(t, mgr, created)
			alias := *created
			alias.SessionID = "session-alias"
			assertCleanupCacheMiss(t, mgr, &alias)
			for _, survivor := range survivors {
				assertCleanupCacheHit(t, mgr, survivor)
				require.DirExists(t, survivor.Path)
				assertCleanupBranchPresent(t, survivor.RepositoryPath, survivor.Branch)
			}
			if !tc.noStore {
				assertWorktreeReferenceStatus(t, store, created.ID, StatusDeleted)
			}
		})
	}
}

func cleanupCacheAliasFixture(t *testing.T) (*Manager, *SQLiteStore, *Worktree, *Worktree) {
	t.Helper()
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-owner", "session-creator", models.TaskSessionStateCompleted)
	created := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-creator")
	_, err := store.db.ExecContext(context.Background(), `
		INSERT INTO repositories (id, workspace_id, name, local_path, default_branch, created_at, updated_at)
		VALUES (?, 'workspace', 'repository', ?, 'main', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		created.RepositoryID, created.RepositoryPath)
	require.NoError(t, err)
	_, err = store.db.ExecContext(context.Background(), `
		INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		VALUES ('session-alias', 'task-owner', 'COMPLETED', 'env-owner', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	projected, err := mgr.GetBySessionAndRepo(context.Background(), "session-alias", created.RepositoryID, created.BranchSlug)
	require.NoError(t, err)
	require.Equal(t, created.ID, projected.ID)
	require.NotEqual(t, created.SessionID, projected.SessionID)
	mgr.worktrees["nil-entry"] = nil
	return mgr, store, created, projected
}

func createCleanupCacheSiblings(t *testing.T, mgr *Manager, store *SQLiteStore, target *Worktree) []*Worktree {
	t.Helper()
	var siblings []*Worktree
	for _, slot := range []struct{ repository, slug string }{
		{repository: "sibling-repository"},
		{repository: target.RepositoryID, slug: "sibling-branch"},
	} {
		wt, err := mgr.Create(context.Background(), CreateRequest{
			TaskID: target.TaskID, SessionID: target.SessionID, TaskTitle: "Sibling",
			RepositoryID: slot.repository, RepositoryPath: target.RepositoryPath,
			BaseBranch: "main", TaskDirName: target.TaskID, RepoName: slot.repository, BranchSlug: slot.slug,
		})
		require.NoError(t, err)
		require.NotEqual(t, target.ID, wt.ID)
		siblings = append(siblings, wt)
	}
	seedReferenceCleanupSession(t, store, "task-unrelated", "session-unrelated", models.TaskSessionStateCompleted)
	unrelated := createReferenceCleanupWorktree(t, mgr, "task-unrelated", "session-unrelated")
	require.Equal(t, target.RepositoryID, unrelated.RepositoryID)
	require.Equal(t, target.BranchSlug, unrelated.BranchSlug)
	require.NotEqual(t, target.ID, unrelated.ID)
	return append(siblings, unrelated)
}

func assertCleanupCacheHit(t *testing.T, mgr *Manager, wt *Worktree) {
	t.Helper()
	mgr.mu.RLock()
	cached := mgr.worktrees[cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug)]
	mgr.mu.RUnlock()
	require.NotNil(t, cached)
	require.Equal(t, wt.ID, cached.ID)
	got, err := mgr.GetBySessionAndRepo(context.Background(), wt.SessionID, wt.RepositoryID, wt.BranchSlug)
	require.NoError(t, err)
	require.Equal(t, wt.ID, got.ID)
}

func assertCleanupCacheMiss(t *testing.T, mgr *Manager, wt *Worktree) {
	t.Helper()
	_, err := mgr.GetBySessionAndRepo(context.Background(), wt.SessionID, wt.RepositoryID, wt.BranchSlug)
	require.ErrorIs(t, err, ErrWorktreeNotFound)
	mgr.mu.RLock()
	_, exists := mgr.worktrees[cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug)]
	mgr.mu.RUnlock()
	require.False(t, exists)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.2
func TestReleaseWorktreeReference_CacheScope(t *testing.T) {
	mgr, store, creator, projected := cleanupCacheAliasFixture(t)
	require.NoError(t, mgr.ReleaseWorktreeReference(context.Background(), projected))
	assertCleanupCacheMiss(t, mgr, projected)
	assertCleanupCacheHit(t, mgr, creator)
	require.DirExists(t, creator.Path)
	assertCleanupBranchPresent(t, creator.RepositoryPath, creator.Branch)
	assertWorktreeReferenceStatus(t, store, creator.ID, StatusDeleted)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.3
func TestCleanupWorktrees_CacheAliasesRetainedOnFailure(t *testing.T) {
	for _, failure := range []string{"dirty checkout", "post-audit path swap", "metadata release", "shared reference"} {
		t.Run(failure, func(t *testing.T) {
			mgr, store, creator, projected := cleanupCacheAliasFixture(t)
			configureCleanupCacheFailure(t, mgr, store, creator, failure)
			var err error
			if failure == "dirty checkout" {
				err = mgr.CleanupWorktrees(context.Background(), []*Worktree{projected})
			} else {
				err = mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{projected})
			}
			if failure == "shared reference" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assertCleanupCacheHit(t, mgr, creator)
			assertCleanupCacheHit(t, mgr, projected)
			assertWorktreeReferenceStatus(t, store, creator.ID, StatusActive)
			assertCleanupBranchPresent(t, creator.RepositoryPath, creator.Branch)
			if failure == "metadata release" {
				require.NoDirExists(t, creator.Path)
			} else {
				require.DirExists(t, creator.Path)
			}
		})
	}
}

func configureCleanupCacheFailure(t *testing.T, mgr *Manager, store *SQLiteStore, creator *Worktree, failure string) {
	t.Helper()
	switch failure {
	case "dirty checkout":
		require.NoError(t, os.WriteFile(filepath.Join(creator.Path, "unique.txt"), []byte("retain"), 0o600))
	case "post-audit path swap":
		mgr.SetRepositoryProvider(&fakeRepoProvider{repo: &Repository{ID: creator.RepositoryID, CleanupScript: "swap"}})
		mgr.SetScriptMessageHandler(&swappingCleanupScriptHandler{})
	case "metadata release":
		mgr.store = &failingReleaseStore{SQLiteStore: store, failUpdate: true}
	case "shared reference":
		seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", models.TaskSessionStateRunning)
		require.NoError(t, store.linkSessionToEnvironment(context.Background(), "session-borrower", "env-owner"))
	}
}

func TestCleanupWorktrees_CacheAliasesEmptyIdentity(t *testing.T) {
	mgr, err := NewManager(newTestConfig(t), nil, newTestLogger())
	require.NoError(t, err)
	target := &Worktree{SessionID: "legacy", RepositoryID: "repository"}
	unrelated := &Worktree{SessionID: "other", RepositoryID: "repository"}
	mgr.worktrees[cacheKey(target.SessionID, target.RepositoryID, "")] = target
	mgr.worktrees[cacheKey(unrelated.SessionID, unrelated.RepositoryID, "")] = unrelated
	mgr.worktrees["nil-entry"] = nil
	require.NoError(t, mgr.finalizeRemovedWorktree(context.Background(), target))
	assertCleanupCacheMiss(t, mgr, target)
	assertCleanupCacheHit(t, mgr, unrelated)
	require.Contains(t, mgr.worktrees, "nil-entry")
}
