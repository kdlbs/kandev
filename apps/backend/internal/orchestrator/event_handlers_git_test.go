package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Regression: when the agent renames or switches branches inside a session,
// handleBranchSwitched must persist the new branch to task_session_worktrees.
// Without this, downstream PR watch reconciliation keeps resolving the stale
// worktree_branch and fails to associate PRs created on the new branch.
func TestHandleBranchSwitched_UpdatesWorktreeBranch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t1", "s1", "step1")

	// Seed a repository + worktree on the original branch.
	rObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := testRepo.CreateRepository(ctx, rObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	wt := &models.TaskSessionWorktree{
		ID: "wt-s1", SessionID: "s1",
		WorktreeID: "wtree-s1", RepositoryID: "repo1",
		WorktreeBranch: "feature/a", CreatedAt: now,
	}
	if err := testRepo.CreateTaskSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/a",
			CurrentBranch:  "feature/b",
			BaseCommit:     "deadbeef",
		},
	})

	// Verify the DB now reflects the new branch.
	wts, err := testRepo.ListTaskSessionWorktrees(ctx, "s1")
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].WorktreeBranch != "feature/b" {
		t.Errorf("WorktreeBranch = %q, want %q", wts[0].WorktreeBranch, "feature/b")
	}
}

// Regression: when a PR watch already exists for the session and the branch
// is switched, the watch must be reset (branch updated, pr_number cleared) so
// the poller re-searches for the PR on the new branch. This covers both
// rename and stacked-PR workflows.
func TestHandleBranchSwitched_ResetsPRWatch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t1", "s1", "step1")

	rObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := testRepo.CreateRepository(ctx, rObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	wt := &models.TaskSessionWorktree{
		ID: "wt-s1", SessionID: "s1",
		WorktreeID: "wtree-s1", RepositoryID: "repo1",
		WorktreeBranch: "feature/a", CreatedAt: now,
	}
	if err := testRepo.CreateTaskSessionWorktree(ctx, wt); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		prWatch: &github.PRWatch{ID: "watch-1", Branch: "feature/a", PRNumber: 42},
	}
	svc.SetGitHubService(ghSvc)

	svc.handleBranchSwitched(ctx, watcher.GitEventData{
		TaskID:    "t1",
		SessionID: "s1",
		BranchSwitch: &lifecycle.GitBranchSwitchData{
			PreviousBranch: "feature/a",
			CurrentBranch:  "feature/b",
			BaseCommit:     "deadbeef",
		},
	})

	if ghSvc.resetWatchCalls != 1 {
		t.Errorf("expected 1 ResetPRWatch call, got %d", ghSvc.resetWatchCalls)
	}
	if ghSvc.resetWatchBranch != "feature/b" {
		t.Errorf("reset watch branch = %q, want feature/b", ghSvc.resetWatchBranch)
	}
}
