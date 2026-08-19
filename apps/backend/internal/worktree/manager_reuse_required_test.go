package worktree

import (
	"context"
	"errors"
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
	before := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))

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
	after := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))
	if after != before {
		t.Fatalf("git worktree list changed during attach-only reuse\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
