package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// archiveDeletesLocalBranch simulates what task archive does to a worktree's
// branch: the local ref is deleted (`git branch -D`), while origin and the
// remote-tracking ref are left alone.
func archiveDeletesLocalBranch(t *testing.T, repoPath, branch string) {
	t.Helper()
	runGit(t, repoPath, "branch", "-D", branch)
}

func newRecreateTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}

// TestRecreate_FetchesBranchFromOriginWhenLocalDeleted is the
// unarchive-recovery path: archive deleted the local branch and the worktree
// directory, but the branch was pushed. recreate must fetch it back from
// origin and rebuild the worktree at the recorded path.
func TestRecreate_FetchesBranchFromOriginWhenLocalDeleted(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	branchSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-1",
		SessionID:      "session-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-1", "repo-1"),
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	wt, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
	})
	if err != nil {
		t.Fatalf("recreate() should fetch the branch from origin, got: %v", err)
	}
	if wt.Status != StatusActive {
		t.Errorf("status = %q, want %q", wt.Status, StatusActive)
	}
	gotSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if gotSHA != branchSHA {
		t.Errorf("worktree HEAD = %q, want %q (pushed branch tip)", gotSHA, branchSHA)
	}
}

func TestRecreate_RequiredBaseRefreshFailureStopsPreparation(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "task-required", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  fetch)
    echo "fatal: Authentication failed" >&2
    exit 1
    ;;
  *)
    exit 0
    ;;
esac
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-required",
		SessionID:      "session-required",
		TaskID:         "task-required",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           worktreePath,
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	_, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:          "session-required",
		TaskID:             "task-required",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		PullBeforeWorktree: true,
	})
	if err == nil {
		t.Fatal("recreate() succeeded after required base refresh failure")
	}
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("required refresh failure removed the existing worktree path: %v", statErr)
	}
}

// TestRecreate_BranchGoneEverywhereReturnsUnrecoverable pins the degraded
// path: local branch deleted AND the branch never made it to origin (or was
// deleted there too). recreate must return ErrBranchUnrecoverable so callers
// can fall back to a fresh worktree instead of failing opaquely.
func TestRecreate_BranchGoneEverywhereReturnsUnrecoverable(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	// Delete the branch everywhere: on origin, locally, and prune the
	// remote-tracking ref so no trace remains.
	runGit(t, repoPath, "push", "origin", "--delete", "feature/pr-branch")
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "fetch", "--prune", "origin")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-2",
		SessionID:      "session-2",
		TaskID:         "task-2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-2", "repo-1"),
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	_, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-2",
		TaskID:         "task-2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
	})
	if !errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("recreate() err = %v, want ErrBranchUnrecoverable", err)
	}
}

// TestBranchRecoveryStatus covers the three probe outcomes used by the
// unarchive HTTP response: local, remote (only the remote-tracking ref
// remains after archive deleted the local branch), and missing.
func TestBranchRecoveryStatus(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr := newRecreateTestManager(t)
	ctx := context.Background()

	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/pr-branch"); got != BranchStatusLocal {
		t.Errorf("status with local branch = %q, want %q", got, BranchStatusLocal)
	}

	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/pr-branch"); got != BranchStatusRemote {
		t.Errorf("status after local delete = %q, want %q", got, BranchStatusRemote)
	}

	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/never-existed"); got != BranchStatusMissing {
		t.Errorf("status for unknown branch = %q, want %q", got, BranchStatusMissing)
	}
	if got := mgr.BranchRecoveryStatus(ctx, "", "feature/pr-branch"); got != BranchStatusMissing {
		t.Errorf("status with empty repo path = %q, want %q", got, BranchStatusMissing)
	}
}

// TestRecreate_ForkPRFetchesPullHeadRef covers fork-PR tasks: the head
// branch never exists on origin by name, only under refs/pull/<N>/head.
// recreate must forward req.PRNumber so fetchBranchToLocal uses the pull
// refspec instead of failing with ErrBranchUnrecoverable.
func TestRecreate_ForkPRFetchesPullHeadRef(t *testing.T) {
	repoPath, prHeadSHA := initGitRepoWithPullRef(t, 974, "feature/fork-pr")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-3",
		SessionID:      "session-3",
		TaskID:         "task-3",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-3", "repo-1"),
		Branch:         "feature/fork-pr",
		Status:         StatusDeleted,
	}

	wt, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-3",
		TaskID:         "task-3",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		PRNumber:       974,
	})
	if err != nil {
		t.Fatalf("recreate() should fetch the fork PR head via pull/<N>/head, got: %v", err)
	}
	gotSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if gotSHA != prHeadSHA {
		t.Errorf("worktree HEAD = %q, want %q (PR head)", gotSHA, prHeadSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRefreshedPRHeadWithoutNetwork(t *testing.T) {
	repoPath, wantSHA := initGitRepoWithPullRef(t, 975, "feature/managed-fork-pr")
	runGit(t, repoPath, "fetch", "origin", "pull/975/head:refs/remotes/origin/pr/975")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-pr", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	refreshCalls := 0
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-pr", SessionID: "session-managed-pr", TaskID: "task-managed-pr",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/managed-fork-pr", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-pr", TaskID: "task-managed-pr", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/managed-fork-pr", PRNumber: 975,
		PullBeforeWorktree: true,
		RefreshRepository: func(context.Context) error {
			refreshCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed PR head %q", got, wantSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRemoteWhenLocalCheckoutBranchIsBehind(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	wantSHA := advanceRemoteBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-branch", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-branch", SessionID: "session-managed-branch", TaskID: "task-managed-branch",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/pr-branch", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-branch", TaskID: "task-managed-branch", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/pr-branch", PullBeforeWorktree: true,
		RefreshRepository: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed branch head %q", got, wantSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRemotePRHeadWhenLocalCheckoutBranchIsBehind(t *testing.T) {
	repoPath, wantSHA := initManagedPRCheckoutBranch(t, 977, "feature/managed-fork-pr-behind")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-pr-behind", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-pr-behind", SessionID: "session-managed-pr-behind", TaskID: "task-managed-pr-behind",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/managed-fork-pr-behind", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-pr-behind", TaskID: "task-managed-pr-behind", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/managed-fork-pr-behind", PRNumber: 977,
		PullBeforeWorktree: true,
		RefreshRepository:  func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed PR head %q", got, wantSHA)
	}
}

// TestCreate_RestoresReleasedWorktreeAfterArchive is the whole unarchive
// round trip at the worktree layer. Archiving a task removes the worktree
// directory and releases its reference (status=deleted + deleted_at) while
// deliberately keeping the git branch, so the next launch must rebuild the
// directory from the released record — including reactivating that record.
// Leaving deleted_at set would hide the restored worktree from every lookup
// that filters on `deleted_at IS NULL`, so the session would silently get a
// brand-new worktree instead of its own work back.
func TestCreate_RestoresReleasedWorktreeAfterArchive(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-archived", "session-archived", models.TaskSessionStateCompleted)

	repoPath := initGitRepoWithRemote(t)
	req := CreateRequest{
		TaskID:         "task-archived",
		SessionID:      "session-archived",
		TaskTitle:      "Archived work",
		RepositoryID:   "repository",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-archived",
		RepoName:       "repository",
	}
	wt, err := mgr.Create(ctx, req)
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	// Work the user never pushed. It only survives because archive keeps the
	// branch (DestroyWorktree passes removeBranch=false).
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "unpushed work")
	workSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	// Archive: remove the directory, release the reference, keep the branch.
	if err := mgr.RemoveByID(ctx, wt.ID, false); err != nil {
		t.Fatalf("archive worktree: %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Fatalf("archive should remove the worktree directory, stat error = %v", statErr)
	}

	// Unarchive + resume: the launch carries the stored worktree ID.
	resumeReq := req
	resumeReq.WorktreeID = wt.ID
	restored, err := mgr.Create(ctx, resumeReq)
	if err != nil {
		t.Fatalf("resume after unarchive must recreate the worktree: %v", err)
	}
	if restored.Path != wt.Path {
		t.Fatalf("restored path = %q, want the original %q", restored.Path, wt.Path)
	}
	if restored.Branch != wt.Branch {
		t.Fatalf("restored branch = %q, want the original %q", restored.Branch, wt.Branch)
	}
	if got := strings.TrimSpace(runGit(t, restored.Path, "rev-parse", "HEAD")); got != workSHA {
		t.Fatalf("restored HEAD = %q, want the pre-archive work %q", got, workSHA)
	}

	// The record must be visible again to the session-scoped lookup the next
	// launch uses; a lingering deleted_at would strand it.
	found, err := store.GetWorktreeBySessionAndRepository(ctx, "session-archived", "repository", restored.BranchSlug)
	if err != nil {
		t.Fatalf("look up restored worktree: %v", err)
	}
	if found == nil {
		t.Fatal("restored worktree is still hidden from the session lookup (deleted_at was not cleared)")
	}
	if found.ID != wt.ID {
		t.Fatalf("session lookup returned worktree %q, want the restored %q", found.ID, wt.ID)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}
