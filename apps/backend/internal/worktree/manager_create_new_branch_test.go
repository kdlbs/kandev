package worktree

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateWorktree_CheckoutBranchAbsentCreatesNewBranchWithName covers the
// MCP add_branch_to_task path: the agent passes a desired CheckoutBranch
// name that doesn't exist yet anywhere. Historically this errored with
// "branch %q not found locally or on remote"; the new contract is "create
// a new branch with that name from baseRef" so the worktree materializes
// without round-tripping through a session restart.
func TestCreateWorktree_CheckoutBranchAbsentCreatesNewBranchWithName(t *testing.T) {
	repoPath := initGitRepoForNewBranchTest(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	desired := "feature/do-nothing-file-a"
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-new-branch",
		SessionID:      "session-new-branch",
		TaskTitle:      "MCP-driven branch addition",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: desired,
		TaskDirName:    "task-new-branch_aaa",
		RepoName:       "repo-1",
		BranchSlug:     SanitizeBranchSlug(desired),
	})
	if err != nil {
		t.Fatalf("Create() with fresh branch name should succeed, got: %v", err)
	}
	if wt.Branch != desired {
		t.Fatalf("worktree.Branch = %q, want %q (caller's desired name)", wt.Branch, desired)
	}
	if !strings.HasSuffix(wt.Path, filepath.Join("task-new-branch_aaa", "repo-1-feature-do-nothing-file-a")) {
		t.Fatalf("worktree path = %q, expected sibling repo-<slug> dir under task root", wt.Path)
	}

	// Branch must now exist locally.
	out := strings.TrimSpace(runGit(t, repoPath, "branch", "--list", desired))
	if out == "" {
		t.Fatalf("expected branch %q to be created locally, got empty branch list", desired)
	}
}

// initGitRepoForNewBranchTest creates a fresh git repo cloned from a bare
// origin so the manager's "branch missing locally and on origin" probe sees
// the desired branch as absent. main is the only branch.
func initGitRepoForNewBranchTest(t *testing.T) string {
	t.Helper()

	bareDir := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, t.TempDir(), "init", "--bare", "-b", "main", bareDir)

	cloneDir := filepath.Join(t.TempDir(), "clone")
	cmd := exec.Command("git", "clone", bareDir, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, out)
	}
	runGit(t, cloneDir, "config", "user.email", "test@example.com")
	runGit(t, cloneDir, "config", "user.name", "Test User")
	runGit(t, cloneDir, "config", "commit.gpgsign", "false")
	runGit(t, cloneDir, "commit", "--allow-empty", "-m", "initial commit")
	runGit(t, cloneDir, "push", "origin", "main")
	return cloneDir
}
