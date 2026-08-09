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
// the owning task environment is deleted, task_environment_repos cascades
// away, so GetByID can no longer resolve a path for that worktree_id.
// RemoveAt must still remove the directory using the caller-supplied
// path/repository handles.
func TestRemoveAt_PathFallbackRemovesDirectoryAfterSessionCascade(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}
	if _, err := mgr.GetByID(ctx, wt.ID); err == nil {
		t.Fatal("expected GetByID to fail once the environment row has cascaded away")
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
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
	// Borrower links its session to the owner's environment, the correct model
	// for a shared-worktree reference in task_environment_repos.
	if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
		t.Fatalf("link session-borrower to env-owner: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("shared worktree path should be preserved: %v", err)
	}
	// In the environment-scoped model there is one task_environment_repos row
	// shared by both sessions; the row remains active because the directory was
	// preserved (RemoveAt returned without calling ReleaseWorktreeReference).
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
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

	// Deleting the environment cascades task_environment_repos, so GetByID
	// fails and RemoveAt takes the path-only fallback with provider resolution.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
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

	// Deleting the environment cascades task_environment_repos, so GetByID
	// fails and RemoveAt takes the path-only fallback — which calls
	// resolveRepositoryPath and must propagate the transient lookup error.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil)
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
	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, "repo-that-does-not-exist", nil); err != nil {
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

// TestRemoveAt_TrackedRow_SameTaskSiblingSessionNoLongerBlocksRemoval is the
// regression test for Review round 3 Finding 1: a task's sessions reuse one
// TaskEnvironment worktree across relaunches, so two sessions on the SAME
// task can each hold their own task_session_worktrees row for the identical
// worktree_id. teardownEnvironmentResources calls RemoveAt exactly ONCE per
// worktree_id (unlike the batch cleanup path, which processes one row per
// session and converges). Before this fix, RemoveAt only excluded the ONE
// session GetByID happened to return, so it saw the sibling session's row as
// an active "foreign" reference, released its own row, and returned without
// ever removing the directory — permanently, since env teardown never
// retries. Passing the whole task's session set must let this single call
// reach the same "no external references, safe to remove" outcome the batch
// path's multi-row convergence produces.
func TestRemoveAt_TrackedRow_SameTaskSiblingSessionNoLongerBlocksRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-a", "completed")
	seedReferenceCleanupSessionOnExistingTask(t, store, "task-owner", "session-b", "completed")

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-a")
	// session-b must reference the same environment so CreateWorktree can
	// resolve its task_environment_id and upsert the shared env-repo row.
	if err := store.linkSessionToEnvironment(ctx, "session-b", "env-owner"); err != nil {
		t.Fatalf("link session-b to env-owner: %v", err)
	}
	sibling := *wt
	sibling.SessionID = "session-b"
	if err := store.CreateWorktree(ctx, &sibling); err != nil {
		t.Fatalf("create sibling session worktree reference: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, []string{"session-a", "session-b"}); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory shared only within the task should be removed, stat error = %v", err)
	}
}

// TestRemoveAt_TrackedRow_ForeignTaskReferenceStillBlocksRemoval guards the
// other side of the same fix: excluding the caller's own task sessions must
// not also swallow a genuinely foreign (different task) active reference.
// Mirrors TestRemoveAt_PreservesSharedActiveReference but drives the
// excludeSessionIDs parameter explicitly, the way teardownEnvironmentResources
// now does, instead of relying on the nil/single-session default.
func TestRemoveAt_TrackedRow_ForeignTaskReferenceStillBlocksRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "running")
	seedReferenceCleanupSession(t, store, "task-borrower", "session-borrower", "running")

	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")
	// Borrower links its session to the owner's environment, the correct model
	// for a shared-worktree reference in task_environment_repos.
	if err := store.linkSessionToEnvironment(ctx, "session-borrower", "env-owner"); err != nil {
		t.Fatalf("link session-borrower to env-owner: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, []string{"session-owner"}); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("worktree still referenced by a different task should be preserved: %v", err)
	}
	// In the environment-scoped model there is one task_environment_repos row
	// shared by both sessions; the row remains active because the directory was
	// preserved (RemoveAt returned without calling ReleaseWorktreeReference).
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

// cleanupScriptTrackingHandler is a ScriptMessageHandler that records every
// cleanup script invocation so tests can assert it ran with the expected
// repository/working-dir, without a real shell.
type cleanupScriptTrackingHandler struct {
	cleanupCalls []ScriptExecutionRequest
}

func (h *cleanupScriptTrackingHandler) ExecuteSetupScript(context.Context, ScriptExecutionRequest) error {
	return nil
}

func (h *cleanupScriptTrackingHandler) ExecuteCleanupScript(_ context.Context, req ScriptExecutionRequest) error {
	h.cleanupCalls = append(h.cleanupCalls, req)
	return nil
}

// TestRemoveAt_PathFallbackRunsRepositoryCleanupScript is the regression test
// for Review round 3 Finding 2: on task delete, the task row (and its
// sessions) are gone by the time the durable cleanup job runs, so RemoveAt's
// path-only fallback is the normal path for every worktree, not just an edge
// case. Before this fix, that fallback removed the directory directly and
// never ran the repository's configured CleanupScript (e.g. `docker compose
// down`), unlike the tracked-row path (removeWorktree) and the batch
// CleanupWorktrees path, both of which always ran it first.
func TestRemoveAt_PathFallbackRunsRepositoryCleanupScript(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	handler := &cleanupScriptTrackingHandler{}
	mgr.SetScriptMessageHandler(handler)
	mgr.SetRepositoryProvider(&stubRepositoryProvider{
		repos: map[string]*Repository{
			wt.RepositoryID: {ID: wt.RepositoryID, LocalPath: wt.RepositoryPath, CleanupScript: "echo cleanup"},
		},
	})

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be removed, stat error = %v", err)
	}
	if len(handler.cleanupCalls) != 1 {
		t.Fatalf("cleanup script calls = %d, want 1", len(handler.cleanupCalls))
	}
	got := handler.cleanupCalls[0]
	if got.RepositoryID != wt.RepositoryID || got.WorkingDir != wt.Path {
		t.Fatalf("cleanup script call = %+v, want RepositoryID=%q WorkingDir=%q", got, wt.RepositoryID, wt.Path)
	}
}

// TestRemoveAt_PathFallbackSkipsCleanupScriptWhenDirectoryAlreadyGone guards
// idempotency: a retried cleanup job must not re-run a repository's cleanup
// script against a directory that no longer exists.
func TestRemoveAt_PathFallbackSkipsCleanupScriptWhenDirectoryAlreadyGone(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	handler := &cleanupScriptTrackingHandler{}
	mgr.SetScriptMessageHandler(handler)
	mgr.SetRepositoryProvider(&stubRepositoryProvider{
		repos: map[string]*Repository{
			wt.RepositoryID: {ID: wt.RepositoryID, LocalPath: wt.RepositoryPath, CleanupScript: "echo cleanup"},
		},
	})

	// Deleting the environment cascades task_environment_repos, so GetByID
	// fails and RemoveAt takes the path-only fallback — which checks pathExists
	// before running the cleanup script.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}
	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree directory ahead of RemoveAt: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
		t.Fatalf("RemoveAt on an already-removed path should be idempotent, got: %v", err)
	}
	if len(handler.cleanupCalls) != 0 {
		t.Fatalf("cleanup script calls = %d, want 0 for an already-removed directory", len(handler.cleanupCalls))
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

// TestRemoveAt_PathFallbackRefusesNonWorktreeDirectory is the regression
// test for Review round 2 Finding 2: TaskEnvironment.WorktreePath is not
// always the worktree directory (see the mainline worktree-launch path,
// which persists it as the TASK ROOT alongside a non-empty worktree_id —
// env_preparer_worktree.go + executor_execute.go). RemoveAt's path-only
// fallback must refuse to os.RemoveAll a path that is not actually a linked
// git worktree, rather than trusting whatever path the caller supplies.
func TestRemoveAt_PathFallbackRefusesNonWorktreeDirectory(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	// Deleting the environment cascades task_environment_repos, so GetByID
	// fails and RemoveAt takes the path-only fallback — which checks IsValid.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	notAWorktree := t.TempDir()
	marker := filepath.Join(notAWorktree, "important.txt")
	if err := os.WriteFile(marker, []byte("do not delete"), 0o644); err != nil {
		t.Fatalf("seed marker file: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, notAWorktree, wt.RepositoryID, nil); err == nil {
		t.Fatal("RemoveAt should refuse to remove a path that is not a linked git worktree")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("non-worktree directory should be preserved, stat error = %v", statErr)
	}
}

// TestRemoveAt_PathFallbackRefusesMainRepositoryCheckout is the sharpest
// instance of the same defect: a main repository checkout (`.git` is a
// directory, not the linked-worktree `gitdir:` file) must never be
// os.RemoveAll'd by the fallback, even though it is exactly the shape
// TaskEnvironment.WorktreePath takes for a multi-repo task root.
func TestRemoveAt_PathFallbackRefusesMainRepositoryCheckout(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	// Deleting the environment cascades task_environment_repos, so GetByID
	// fails and RemoveAt takes the path-only fallback — which checks IsValid.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, "env-owner"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.RepositoryPath, wt.RepositoryID, nil); err == nil {
		t.Fatal("RemoveAt should refuse to remove a main repository checkout")
	}
	if _, statErr := os.Stat(filepath.Join(wt.RepositoryPath, ".git")); statErr != nil {
		t.Fatalf("main repository checkout should be preserved, stat error = %v", statErr)
	}
}

// TestRemoveAt_PathFallbackTreatsAlreadyRemovedDirectoryAsSuccess confirms
// the validation guard does not regress idempotency: a worktree directory
// that is already gone (e.g. a retried cleanup job) must still report
// success, not get misclassified as "not a worktree".
func TestRemoveAt_PathFallbackTreatsAlreadyRemovedDirectoryAsSuccess(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-owner", "session-owner", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-owner", "session-owner")

	if _, err := store.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-owner"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree directory ahead of RemoveAt: %v", err)
	}

	if err := mgr.RemoveAt(ctx, wt.ID, wt.Path, wt.RepositoryID, nil); err != nil {
		t.Fatalf("RemoveAt on an already-removed path should be idempotent, got: %v", err)
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
