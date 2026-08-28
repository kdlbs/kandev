package worktree

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCleanupWorktrees_PreservesBranchWhenRequested(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-archive", "session-archive", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-archive", "session-archive")
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "local-only archive work")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("branch-preserving cleanup: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree path remains after cleanup, stat error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", wt.Branch)); got != wantHead {
		t.Fatalf("preserved branch head = %q, want %q", got, wantHead)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_RemovesBranchByDefault(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-delete", "session-delete", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-delete", "session-delete")

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("default cleanup preserved branch %q", got)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}
