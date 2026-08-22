package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate_ReuseRequiredRejectsMissingCanonicalWorktree(t *testing.T) {
	mgr := newRecreateTestManager(t)

	_, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-2",
		RepositoryID:   "repository-1",
		RepositoryPath: t.TempDir(),
		BaseBranch:     "main",
		WorktreeID:     "canonical-worktree",
		ReuseRequired:  true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() error = %v, want ErrReuseWorktreeUnavailable", err)
	}
}

func TestCreate_ReuseRequiredReturnsCanonicalWorktreeWithoutChangingGitState(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "canonical")
	runGit(t, repoPath, "worktree", "add", "-b", "reuse-required", worktreePath, "main")
	marker := filepath.Join(worktreePath, "session-a-uncommitted-marker")
	if err := os.WriteFile(marker, []byte("shared workspace state\n"), 0o600); err != nil {
		t.Fatalf("write uncommitted marker: %v", err)
	}
	before := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))
	beforeBranch := strings.TrimSpace(runGit(t, worktreePath, "branch", "--show-current"))
	beforeHead := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD"))
	beforeStatus := strings.TrimSpace(runGit(t, worktreePath, "status", "--porcelain"))

	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-1",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "reuse-required",
		Status:            StatusActive,
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-1",
		SessionID:         "session-2",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-worktree",
		ReuseRequired:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "canonical-worktree" || got.Path != worktreePath {
		t.Fatalf("Create() = %#v, want canonical worktree", got)
	}
	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-3",
		RepositoryID:   "repository-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		WorktreeID:     "canonical-worktree",
		ReuseRequired:  true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() without TaskEnvironmentID error = %v, want ErrReuseWorktreeUnavailable", err)
	}
	after := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))
	if after != before {
		t.Fatalf("git worktree list changed during attach-only reuse\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if branch := strings.TrimSpace(runGit(t, worktreePath, "branch", "--show-current")); branch != beforeBranch {
		t.Fatalf("branch changed during attach-only reuse: before=%q after=%q", beforeBranch, branch)
	}
	if head := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD")); head != beforeHead {
		t.Fatalf("HEAD changed during attach-only reuse: before=%q after=%q", beforeHead, head)
	}
	if status := strings.TrimSpace(runGit(t, worktreePath, "status", "--porcelain")); status != beforeStatus {
		t.Fatalf("status changed during attach-only reuse: before=%q after=%q", beforeStatus, status)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("attach-only reuse lost uncommitted marker: %v", err)
	}
}

func TestCreate_ReuseRequiredRejectsCanonicalWorktreeFromAnotherBranch(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "canonical")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/one", worktreePath, "main")
	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-1",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "feature/one",
		BranchSlug:        "feature-one",
		Status:            StatusActive,
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-1",
		SessionID:          "session-2",
		TaskEnvironmentID:  "environment-1",
		RepositoryID:       "repository-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		WorktreeID:         "canonical-worktree",
		BranchIdentitySlug: "feature-two",
		ReuseRequired:      true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() error = %v, want ErrReuseWorktreeUnavailable", err)
	}
}

func TestCreate_ReuseRequiredAllowsAuthorizedEnvironmentFromAnotherTask(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "canonical")
	runGit(t, repoPath, "worktree", "add", "-b", "shared-environment", worktreePath, "main")
	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "owner-task",
		TaskEnvironmentID: "shared-environment",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "shared-environment",
		Status:            StatusActive,
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "inherited-task",
		SessionID:         "session-2",
		TaskEnvironmentID: "shared-environment",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-worktree",
		ReuseRequired:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want inherited task to attach: %v", err, err)
	}
	if got.ID != "canonical-worktree" {
		t.Fatalf("Create() worktree ID = %q, want canonical-worktree", got.ID)
	}
}
