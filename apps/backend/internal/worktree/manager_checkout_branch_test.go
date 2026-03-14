package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktree_CheckoutBranchDirectCheckout(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoForWorktreeTest(t)

	// First worktree with checkout branch should use the branch directly.
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "PR Review",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if wt.Branch != "feature/pr-branch" {
		t.Fatalf("expected direct checkout branch %q, got %q", "feature/pr-branch", wt.Branch)
	}
	if wt.FetchWarning == "" {
		t.Fatal("expected fetch warning when origin is unavailable and local branch is reused")
	}

	gotBranch := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch != "feature/pr-branch" {
		t.Fatalf("worktree HEAD branch = %q, want %q", gotBranch, "feature/pr-branch")
	}

	prHeadSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))
	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != prHeadSHA {
		t.Fatalf("worktree HEAD SHA = %q, want %q", worktreeSHA, prHeadSHA)
	}
}

func TestCreateWorktree_CheckoutBranchFallsBackToSuffix(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoForWorktreeTest(t)

	// First worktree checks out the branch directly.
	wt1, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "PR Review 1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
	})
	if err != nil {
		t.Fatalf("Create() first worktree: %v", err)
	}
	if wt1.Branch != "feature/pr-branch" {
		t.Fatalf("first worktree: expected direct branch, got %q", wt1.Branch)
	}

	// Second worktree with the same checkout branch should fall back to a suffixed name.
	wt2, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-2",
		SessionID:      "session-2",
		TaskTitle:      "PR Review 2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
	})
	if err != nil {
		t.Fatalf("Create() second worktree: %v", err)
	}
	if wt2.Branch == "feature/pr-branch" {
		t.Fatal("second worktree should NOT use the checkout branch directly")
	}
	if !strings.HasPrefix(wt2.Branch, "feature/pr-branch-") {
		t.Fatalf("expected suffixed branch like %q, got %q", "feature/pr-branch-xxx", wt2.Branch)
	}

	// Both worktrees should point to the same commit.
	sha1 := strings.TrimSpace(runGit(t, wt1.Path, "rev-parse", "HEAD"))
	sha2 := strings.TrimSpace(runGit(t, wt2.Path, "rev-parse", "HEAD"))
	if sha1 != sha2 {
		t.Fatalf("worktree SHAs differ: wt1=%q, wt2=%q", sha1, sha2)
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
