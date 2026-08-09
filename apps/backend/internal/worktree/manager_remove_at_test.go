package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// stubRepositoryProvider is a minimal RepositoryProvider for exercising
// resolveRepositoryPath/RemoveAt's repository-resolution branches without
// wiring a full task repository.
type stubRepositoryProvider struct {
	repos map[string]*Repository
	err   error
}

func (p *stubRepositoryProvider) GetRepository(_ context.Context, repositoryID string) (*Repository, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.repos[repositoryID], nil
}

// TestManager_ResolveRepositoryPath covers every branch of Review Finding
// 1's core distinction: a repository that's genuinely unresolvable (no
// provider, no ID, not found, or missing a local path) is a permanent
// condition — resolvable=false, err=nil — while any other lookup failure is
// transient and must be returned so the caller can retry instead of
// silently degrading.
func TestManager_ResolveRepositoryPath(t *testing.T) {
	lookupErr := errors.New("db unavailable")
	tests := []struct {
		name         string
		provider     RepositoryProvider
		repositoryID string
		wantPath     string
		wantResolved bool
		wantErr      error
	}{
		{name: "no provider wired", provider: nil, repositoryID: "repo-a"},
		{name: "empty repository id", provider: &stubRepositoryProvider{}, repositoryID: ""},
		{
			name:         "repository not found is permanent, not an error",
			provider:     &stubRepositoryProvider{err: repoerrors.ErrRepositoryNotFound},
			repositoryID: "repo-a",
		},
		{
			name:         "repository has no local path",
			provider:     &stubRepositoryProvider{repos: map[string]*Repository{"repo-a": {ID: "repo-a"}}},
			repositoryID: "repo-a",
		},
		{
			name:         "transient lookup failure is returned, not swallowed",
			provider:     &stubRepositoryProvider{err: lookupErr},
			repositoryID: "repo-a",
			wantErr:      lookupErr,
		},
		{
			name:         "resolves the repository local path",
			provider:     &stubRepositoryProvider{repos: map[string]*Repository{"repo-a": {ID: "repo-a", LocalPath: "/repos/repo-a"}}},
			repositoryID: "repo-a",
			wantPath:     "/repos/repo-a",
			wantResolved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &Manager{logger: newTestLogger(), repoProvider: tt.provider}
			path, resolved, err := mgr.resolveRepositoryPath(context.Background(), tt.repositoryID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want it to wrap %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved != tt.wantResolved || path != tt.wantPath {
				t.Fatalf("resolveRepositoryPath() = (%q, %v), want (%q, %v)", path, resolved, tt.wantPath, tt.wantResolved)
			}
		})
	}
}

// TestRemoveAt_PathFallbackRemovesDirectoryAfterSessionCascade is the
// worktree-package-level regression test for the multi-repo disk leak: once
// the owning session is deleted, task_session_worktrees — the only table
// that stores a worktree's on-disk path for an ID-only lookup — cascades
// away, so GetByID can no longer resolve a path for that worktree_id.
// RemoveAt must still remove the directory using the caller-supplied
// path/repository handles.
func TestRemoveAt_PathFallbackRemovesDirectoryAfterSessionCascade(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := mgr.GetByID(ctx, wt.ID); err == nil {
		t.Fatal("expected GetByID to fail once the session-scoped row has cascaded away")
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be removed, stat error = %v", err)
	}
}

// TestRemoveAt_PreservesSharedActiveReference confirms the safety ordering:
// when the worktree_id still resolves through GetByID (the owner's row is
// still present, and another session's row references the same physical
// worktree), RemoveAt must delegate to the same tracked-row path RemoveByID
// uses — never take the path-only fallback — so the shared/borrowed-worktree
// reference count guard is never bypassed.
func TestRemoveAt_PreservesSharedActiveReference(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "running")
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", "running")

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	borrowed := *wt
	borrowed.TaskID = "task-borrower"
	borrowed.SessionID = "session-borrower"
	if err := store.CreateWorktree(ctx, &borrowed); err != nil {
		t.Fatalf("create borrower worktree reference: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-owner", StatusDeleted)
	assertWorktreeReferenceStatus(t, store, wt.ID, "session-borrower", StatusActive)
}

// TestRemoveAt_PathFallbackPrunesSourceRepositoryRegistration is the positive
// counterpart of the session-cascade test above: when a repository provider
// IS wired and resolves the worktree's actual source repository, RemoveAt's
// fallback must scope `git worktree remove`/`prune` to that repository so no
// stale registration is left behind (Review Finding 1).
func TestRemoveAt_PathFallbackPrunesSourceRepositoryRegistration(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	mgr.SetRepositoryProvider(&stubRepositoryProvider{
		repos: map[string]*Repository{wt.RepositoryID: {ID: wt.RepositoryID, LocalPath: wt.RepositoryPath}},
	})

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be removed, stat error = %v", err)
	}
	assertNoWorktreeRegistration(t, wt.RepositoryPath, wt.Path)
}

// TestRemoveAt_PropagatesTransientRepositoryLookupFailure is the regression
// test for the other half of Review Finding 1: resolveRepositoryPath used to
// collapse every GetRepository error — including a transient one — to "",
// so RemoveAt would remove the directory and report success even though it
// never learned the real repository path. A transient failure must instead
// be returned so the durable cleanup job retries, leaving the directory (and
// the caller's ability to find it again) intact.
func TestRemoveAt_PropagatesTransientRepositoryLookupFailure(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	lookupErr := errors.New("db unavailable")
	mgr.SetRepositoryProvider(&stubRepositoryProvider{err: lookupErr})

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID)
	if !errors.Is(err, lookupErr) {
		t.Fatalf("RemoveAt() error = %v, want it to wrap %v so the caller retries instead of silently degrading", err, lookupErr)
	}
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Fatalf("worktree directory should be preserved when the lookup fails so a retry can still find it: %v", statErr)
	}
}

// TestRemoveAt_PathFallbackUnresolvableRepositoryNeverTouchesAmbientDirectory
// is the regression test for the core half of Review Finding 1: exec.Cmd
// treats an empty Dir as the calling process's own working directory. If
// RemoveAt's fallback ever runs `git worktree remove`/`prune` with an
// unresolved (empty) repository path, it silently operates on whatever
// repository happens to be the process's cwd instead of the worktree's
// actual source repo. This points cwd at an isolated decoy repo — never the
// real dev checkout this suite runs inside of — that carries its own
// dangling worktree registration, so an accidental cross-repo git invocation
// is directly observable: pre-fix, it gets silently pruned as a side effect
// of removing a worktree the decoy repo has never heard of.
func TestRemoveAt_PathFallbackUnresolvableRepositoryNeverTouchesAmbientDirectory(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	decoyRepo, danglingWorktreePath := newDecoyRepoWithDanglingWorktree(t)
	t.Chdir(decoyRepo)

	// No repository provider wired: repositoryID cannot resolve, exactly the
	// "genuinely unresolvable" case removeWorktreeDir must not run git for.
	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, "repo-that-does-not-exist"); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be removed, stat error = %v", err)
	}
	out := runGit(t, decoyRepo, "worktree", "list", "--porcelain")
	if !strings.Contains(out, danglingWorktreePath) {
		t.Fatalf("decoy repository's unrelated dangling worktree registration was pruned by RemoveAt "+
			"(git ran against the wrong repository — an empty exec.Cmd.Dir resolves to the process's cwd):\n%s", out)
	}
}

// assertNoWorktreeRegistration fails the test if repoPath's own `git
// worktree list` still references worktreePath.
func assertNoWorktreeRegistration(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	out := runGit(t, repoPath, "worktree", "list", "--porcelain")
	if strings.Contains(out, worktreePath) {
		t.Fatalf("stale worktree registration for %s remains in source repo %s:\n%s", worktreePath, repoPath, out)
	}
}

// newDecoyRepoWithDanglingWorktree creates a standalone git repo with a
// worktree whose directory was removed without running `git worktree
// remove`/`prune`, leaving a `prunable` registration — an observable marker
// that survives untouched unless something runs `git worktree prune` (or
// `remove`) inside this repo.
func newDecoyRepoWithDanglingWorktree(t *testing.T) (repoPath, worktreePath string) {
	t.Helper()
	repoPath = filepath.Join(t.TempDir(), "decoy")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir decoy repo: %v", err)
	}
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	readme := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readme, []byte("decoy\n"), 0o644); err != nil {
		t.Fatalf("write decoy README: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")

	worktreePath = filepath.Join(t.TempDir(), "decoy-worktree")
	runGit(t, repoPath, "worktree", "add", "-b", "decoy-branch", worktreePath)
	if err := os.RemoveAll(worktreePath); err != nil {
		t.Fatalf("remove decoy worktree directory without pruning: %v", err)
	}
	return repoPath, worktreePath
}
