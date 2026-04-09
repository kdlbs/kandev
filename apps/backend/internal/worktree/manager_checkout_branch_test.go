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

// TestCreateWorktree_CheckoutBranchNoFetchWarningWithRemote verifies that when
// a branch is checked out in one worktree, creating a second worktree for the
// same branch does NOT produce a fetch warning. The fetch retries with a
// remote-tracking ref update instead of trying to update the checked-out local branch.
func TestCreateWorktree_CheckoutBranchNoFetchWarningWithRemote(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

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
	if wt1.FetchWarning != "" {
		t.Fatalf("first worktree: unexpected fetch warning: %s", wt1.FetchWarning)
	}

	// Second worktree with the same checkout branch should fall back to a
	// suffixed name but should NOT produce a fetch warning — the fetch retries
	// via remote-tracking ref when the local branch is checked out.
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
	if wt2.FetchWarning != "" {
		t.Fatalf("second worktree: unexpected fetch warning: %s", wt2.FetchWarning)
	}

	// Both worktrees should point to the same commit (latest from origin).
	sha1 := strings.TrimSpace(runGit(t, wt1.Path, "rev-parse", "HEAD"))
	sha2 := strings.TrimSpace(runGit(t, wt2.Path, "rev-parse", "HEAD"))
	if sha1 != sha2 {
		t.Fatalf("worktree SHAs differ: wt1=%q, wt2=%q", sha1, sha2)
	}

	// Both worktrees should have upstream set to origin/feature/pr-branch
	// so that "git rev-list branch...@{upstream}" gives correct ahead/behind counts.
	const wantUpstream = "origin/feature/pr-branch"
	upstream1 := strings.TrimSpace(runGit(t, wt1.Path, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if upstream1 != wantUpstream {
		t.Fatalf("first worktree upstream = %q, want %q", upstream1, wantUpstream)
	}
	upstream2 := strings.TrimSpace(runGit(t, wt2.Path, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if upstream2 != wantUpstream {
		t.Fatalf("second worktree upstream = %q, want %q", upstream2, wantUpstream)
	}
}

// TestCreateWorktree_RemoteBaseRefDoesNotSetUpstream verifies that when a worktree
// is created with a remote-tracking ref as the base (e.g. origin/feature/foo),
// the new branch does NOT inherit upstream tracking from that ref.
// This was a bug: git auto-sets upstream when branching from a remote ref,
// causing the new task branch to track the original remote branch.
func TestCreateWorktree_RemoteBaseRefDoesNotSetUpstream(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	// Create a worktree using a remote-tracking ref as the base branch.
	// This simulates the user selecting a remote branch in the UI, or
	// PullBeforeWorktree resolving to origin/feature/pr-branch.
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "New Feature",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "origin/feature/pr-branch",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// The new branch should NOT have upstream tracking set.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}")
	cmd.Dir = wt.Path
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected no upstream for new task branch, but got %q", strings.TrimSpace(string(out)))
	}

	// The branch should still be based on the correct commit.
	featureSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "origin/feature/pr-branch"))
	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != featureSHA {
		t.Fatalf("worktree HEAD SHA = %q, want %q (origin/feature/pr-branch)", worktreeSHA, featureSHA)
	}
}

// initGitRepoWithRemote creates a bare "origin" repo, clones it, and creates
// a feature branch with a commit. Returns the clone path (has origin remote).
func initGitRepoWithRemote(t *testing.T) string {
	t.Helper()

	// Create a bare repo to serve as "origin"
	bareDir := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, t.TempDir(), "init", "--bare", "-b", "main", bareDir)

	// Clone it to get a working repo with origin
	cloneDir := filepath.Join(t.TempDir(), "clone")
	cmd := exec.Command("git", "clone", bareDir, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, out)
	}
	runGit(t, cloneDir, "config", "user.email", "test@example.com")
	runGit(t, cloneDir, "config", "user.name", "Test User")
	runGit(t, cloneDir, "config", "commit.gpgsign", "false")

	// Create initial commit on main
	filePath := filepath.Join(cloneDir, "README.md")
	if err := os.WriteFile(filePath, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	runGit(t, cloneDir, "add", "README.md")
	runGit(t, cloneDir, "commit", "-m", "initial commit")
	runGit(t, cloneDir, "push", "origin", "main")

	// Create feature branch with a commit
	runGit(t, cloneDir, "checkout", "-b", "feature/pr-branch")
	if err := os.WriteFile(filePath, []byte("feature change\n"), 0644); err != nil {
		t.Fatalf("failed to update README.md: %v", err)
	}
	runGit(t, cloneDir, "add", "README.md")
	runGit(t, cloneDir, "commit", "-m", "feature commit")
	runGit(t, cloneDir, "push", "origin", "feature/pr-branch")

	// Go back to main so the branch isn't checked out in the main repo
	runGit(t, cloneDir, "checkout", "main")

	return cloneDir
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
