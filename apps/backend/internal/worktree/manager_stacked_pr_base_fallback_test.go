package worktree

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestCreateWorktree_MissingRemoteBaseUsesRefreshedFallback(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	var events []SyncProgressEvent

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-stacked-pr",
		SessionID:          "session-stacked-pr",
		TaskTitle:          "Review stacked PR",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "feature/deleted-parent",
		FallbackBaseBranch: "main",
		CheckoutBranch:     "feature/pr-branch",
		PullBeforeWorktree: true,
		TaskDirName:        "task-stacked-pr",
		RepoName:           "repo-1",
		OnSyncProgress:     captureSyncProgress(&events),
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree BaseBranch = %q, want main", wt.BaseBranch)
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "feature/deleted-parent") ||
		!strings.Contains(wt.BaseBranchFallbackWarning, "main") {
		t.Fatalf("fallback warning = %q, want old and new branches", wt.BaseBranchFallbackWarning)
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "no longer exists on origin") {
		t.Fatalf("fallback warning = %q, want missing-origin explanation", wt.BaseBranchFallbackWarning)
	}
	wantHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "origin/feature/pr-branch"))
	gotHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if gotHead != wantHead {
		t.Fatalf("worktree HEAD = %s, want checkout branch head %s", gotHead, wantHead)
	}
	for _, event := range events {
		if event.Status == SyncProgressFailed {
			t.Fatalf("sync progress contains failed event: %#v", events)
		}
	}
	if len(events) == 0 || events[len(events)-1].Status != SyncProgressCompleted {
		t.Fatalf("final sync progress = %#v, want completed", events)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestCreateWorktree_MissingRemoteBaseWithoutFallbackReportsMissingRef(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	var events []SyncProgressEvent

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-stacked-pr",
		SessionID:          "session-stacked-pr",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "feature/deleted-parent",
		CheckoutBranch:     "feature/pr-branch",
		PullBeforeWorktree: true,
		TaskDirName:        "task-stacked-pr",
		RepoName:           "repo-1",
		OnSyncProgress:     captureSyncProgress(&events),
	})
	if err == nil {
		t.Fatal("Create() succeeded without a fallback")
	}
	if !strings.Contains(err.Error(), "missing_remote_ref") {
		t.Fatalf("Create() error = %q, want missing_remote_ref", err)
	}
	if len(events) == 0 || events[len(events)-1].Status != SyncProgressFailed ||
		events[len(events)-1].Error != "missing_remote_ref" {
		t.Fatalf("final sync progress = %#v, want missing_remote_ref failure", events)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestCreateWorktree_MissingRemoteBaseAndFallbackReportsMissingRef(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-stacked-pr",
		SessionID:          "session-stacked-pr",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "feature/deleted-parent",
		FallbackBaseBranch: "feature/deleted-fallback",
		CheckoutBranch:     "feature/pr-branch",
		PullBeforeWorktree: true,
		TaskDirName:        "task-stacked-pr",
		RepoName:           "repo-1",
	})
	if err == nil {
		t.Fatal("Create() succeeded with a missing fallback")
	}
	if !strings.Contains(err.Error(), "missing_remote_ref") {
		t.Fatalf("Create() error = %q, want missing_remote_ref", err)
	}
	if errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("Create() error = %v, want required-refresh failure", err)
	}
}
