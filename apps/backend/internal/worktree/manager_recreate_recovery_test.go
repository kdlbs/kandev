package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
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
