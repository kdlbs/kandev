package worktree

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestCreateWorktree_BaseBranchFallback_RecoversWhenRequestedMissing covers
// the regression where a subtask inherited a stale base_branch (e.g. the
// parent's PR head) that did not exist in a freshly inherited worktree. With
// FallbackBaseBranch populated (typically the repo's default_branch) the
// manager retries with the fallback and surfaces a non-fatal warning instead
// of failing the launch.
func TestCreateWorktree_BaseBranchFallback_RecoversWhenRequestedMissing(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := initGitRepoForWorktreeTest(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-1",
		SessionID:          "session-1",
		TaskTitle:          "Inherits stale branch",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "pr-metrics",
		FallbackBaseBranch: "main",
		TaskDirName:        "task-1",
		RepoName:           "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree BaseBranch = %q, want %q", wt.BaseBranch, "main")
	}
	if wt.BaseBranchFallbackWarning == "" {
		t.Fatal("expected BaseBranchFallbackWarning to be set when fallback was used")
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "pr-metrics") {
		t.Fatalf("warning %q does not mention requested branch", wt.BaseBranchFallbackWarning)
	}
	if !strings.Contains(wt.BaseBranchFallbackWarning, "main") {
		t.Fatalf("warning %q does not mention fallback branch", wt.BaseBranchFallbackWarning)
	}
	if wt.BaseBranchFallbackDetail == "" {
		t.Fatal("expected BaseBranchFallbackDetail to be set when fallback was used")
	}
}

// TestCreateWorktree_BaseBranchFallback_NoFallbackProvided keeps the legacy
// loud failure when the caller did not supply a FallbackBaseBranch — we must
// not silently invent a branch out of thin air.
func TestCreateWorktree_BaseBranchFallback_NoFallbackProvided(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := initGitRepoForWorktreeTest(t)

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "pr-metrics",
		TaskDirName:    "task-1",
		RepoName:       "repo-1",
	})
	if !errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("Create() err = %v, want ErrInvalidBaseBranch", err)
	}
}

// TestCreateWorktree_BaseBranchFallback_FallbackAlsoMissing surfaces the
// original error when neither branch exists — the fallback only helps when it
// actually exists in the repository.
func TestCreateWorktree_BaseBranchFallback_FallbackAlsoMissing(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := initGitRepoForWorktreeTest(t)

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-1",
		SessionID:          "session-1",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "pr-metrics",
		FallbackBaseBranch: "release", // also missing in this repo
		TaskDirName:        "task-1",
		RepoName:           "repo-1",
	})
	if !errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("Create() err = %v, want ErrInvalidBaseBranch", err)
	}
	// The error must name both branches so an operator can tell at a glance
	// that a fallback was attempted and also failed.
	msg := err.Error()
	if !strings.Contains(msg, "pr-metrics") || !strings.Contains(msg, "release") {
		t.Fatalf("error %q should mention both requested and fallback branches", msg)
	}
}

// TestCreateWorktree_BaseBranchFallback_NotUsedWhenBaseExists guards against
// the fallback being applied unnecessarily — when the requested branch is
// valid the worktree uses it and no warning is emitted.
func TestCreateWorktree_BaseBranchFallback_NotUsedWhenBaseExists(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := initGitRepoForWorktreeTest(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-1",
		SessionID:          "session-1",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		FallbackBaseBranch: "feature/pr-branch", // exists, but should never be used
		TaskDirName:        "task-1",
		RepoName:           "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree BaseBranch = %q, want %q", wt.BaseBranch, "main")
	}
	if wt.BaseBranchFallbackWarning != "" {
		t.Fatalf("expected no fallback warning, got %q", wt.BaseBranchFallbackWarning)
	}
}
