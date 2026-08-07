package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestRecreate_SerializesAgainstRepoLock covers the second half of the
// review-round-1 lock finding (docs/specs/session-delete-resource-cleanup):
// recreate()'s cleanup of an existing worktree directory (os.RemoveAll +
// `git worktree prune`) used to run before acquiring any lock at all, so it
// could race a concurrent ReclaimSessionWorktree call for the worktree being
// replaced — both racing to delete/inspect the same path outside any shared
// lock. It must now run only while holding repoLock, the same lock
// ReclaimSessionWorktree holds across its own critical section.
func TestRecreate_SerializesAgainstRepoLock(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr := newRecreateTestManager(t)

	existingPath := filepath.Join(t.TempDir(), "task-1", "repo-1")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("mkdir existing path: %v", err)
	}
	sentinel := filepath.Join(existingPath, "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatalf("write sentinel file: %v", err)
	}
	existing := &Worktree{
		ID:             "wt-1",
		SessionID:      "session-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           existingPath,
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	repoLock := mgr.getRepoLock(repoPath)
	repoLock.Lock()

	done := make(chan error, 1)
	go func() {
		_, err := mgr.recreate(context.Background(), existing, CreateRequest{
			SessionID:      "session-1",
			TaskID:         "task-1",
			RepositoryID:   "repo-1",
			RepositoryPath: repoPath,
		})
		done <- err
	}()

	// While we hold repoLock externally, recreate() must not have touched
	// the existing directory yet — proven by the sentinel file surviving.
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("recreate() removed the existing directory before acquiring repoLock (sentinel gone): %v", err)
	}

	repoLock.Unlock()
	mgr.releaseRepoLock(repoPath)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recreate() after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recreate() did not complete after repoLock was released")
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("recreate() should have removed the existing directory once it acquired repoLock, stat error = %v", err)
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
