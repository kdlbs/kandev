package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktree_CheckoutBranchUsesUniqueLocalBranch(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoForWorktreeTest(t)
	addedWorktreePath := filepath.Join(t.TempDir(), "existing-pr-worktree")
	runGit(t, repoPath, "worktree", "add", addedWorktreePath, "feature/pr-branch")

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "PR #9278 Test Playback",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.Branch == "feature/pr-branch" {
		t.Fatalf("expected unique worktree branch, got checkout branch %q", wt.Branch)
	}
	if wt.FetchWarning == "" {
		t.Fatal("expected fetch warning when origin is unavailable and local branch is reused")
	}

	gotBranch := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch != wt.Branch {
		t.Fatalf("worktree HEAD branch = %q, want %q", gotBranch, wt.Branch)
	}

	prHeadSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))
	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != prHeadSHA {
		t.Fatalf("worktree HEAD SHA = %q, want %q", worktreeSHA, prHeadSHA)
	}
}

func initGitRepoForWorktreeTest(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "branch", "feature/pr-branch")

	return repoPath
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}
