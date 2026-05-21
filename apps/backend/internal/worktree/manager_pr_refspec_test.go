package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFetchBranchToLocal_PRNumberUsesPullRefspec verifies that when a PR
// number is supplied, the manager fetches refs/pull/<N>/head into the local
// branch. This is the fork-PR path: the head branch does not exist on origin
// by name (only as a pull/<N>/head ref), so the previous branch-name fetch
// would fail with "couldn't find remote ref".
func TestFetchBranchToLocal_PRNumberUsesPullRefspec(t *testing.T) {
	repoPath, prHeadSHA := initGitRepoWithPullRef(t, 974, "feature/enrich-linear-issue-hap")

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result, err := mgr.fetchBranchToLocal(
		context.Background(), repoPath, "feature/enrich-linear-issue-hap", 974,
	)
	if err != nil {
		t.Fatalf("fetchBranchToLocal(pr=974) unexpected error: %v", err)
	}
	if result.Warning != "" {
		t.Fatalf("expected no warning, got %q", result.Warning)
	}

	gotSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/enrich-linear-issue-hap"))
	if gotSHA != prHeadSHA {
		t.Fatalf("local branch SHA = %q, want %q (PR head)", gotSHA, prHeadSHA)
	}
}

// TestFetchBranchToLocal_PRNumberZeroUsesBranchFetch confirms the legacy path
// is preserved when PRNumber == 0: the manager fetches origin/<branch>, which
// is the correct behavior for non-PR tasks and same-repo branches that the
// caller knows live on origin under the same name.
func TestFetchBranchToLocal_PRNumberZeroUsesBranchFetch(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result, err := mgr.fetchBranchToLocal(
		context.Background(), repoPath, "feature/pr-branch", 0,
	)
	if err != nil {
		t.Fatalf("fetchBranchToLocal(pr=0) unexpected error: %v", err)
	}
	if result.Warning != "" {
		t.Fatalf("expected no warning, got %q", result.Warning)
	}
}

// TestCreateWorktree_PRNumberCreatesWorktreeFromForkRef is the end-to-end test
// for fork PRs: only the pull/<N>/head ref exists on origin (no branch named
// after the head ref). Worktree creation must still succeed by using the PR
// refspec internally, and the worktree must point at the PR head commit.
func TestCreateWorktree_PRNumberCreatesWorktreeFromForkRef(t *testing.T) {
	repoPath, prHeadSHA := initGitRepoWithPullRef(t, 974, "feature/enrich-linear-issue-hap")

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-fork-pr",
		SessionID:      "session-fork-pr",
		TaskTitle:      "Fork PR review",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/enrich-linear-issue-hap",
		PRNumber:       974,
		TaskDirName:    "task-fork-pr",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() with PRNumber should succeed for fork PR, got: %v", err)
	}
	if wt.Branch != "feature/enrich-linear-issue-hap" {
		t.Fatalf("expected branch %q, got %q", "feature/enrich-linear-issue-hap", wt.Branch)
	}

	worktreeSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if worktreeSHA != prHeadSHA {
		t.Fatalf("worktree HEAD SHA = %q, want %q (PR head)", worktreeSHA, prHeadSHA)
	}
}

// initGitRepoWithPullRef simulates a fork PR scenario: a bare origin repo
// containing main + an arbitrary commit registered under refs/pull/<N>/head
// (but NOT under any branch). Returns the clone path and the PR head SHA.
func initGitRepoWithPullRef(t *testing.T, prNumber int, headBranchName string) (string, string) {
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

	filePath := filepath.Join(cloneDir, "README.md")
	if err := os.WriteFile(filePath, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	runGit(t, cloneDir, "add", "README.md")
	runGit(t, cloneDir, "commit", "-m", "initial commit")
	runGit(t, cloneDir, "push", "origin", "main")

	// Build the PR head commit on a temporary branch, then publish it ONLY
	// under refs/pull/<N>/head on the bare origin. The branch name the
	// contributor used does not exist on origin — matching the fork PR
	// scenario where the head branch lives only on the contributor's fork.
	runGit(t, cloneDir, "checkout", "-b", "tmp-pr-head")
	if err := os.WriteFile(filePath, []byte("pr change\n"), 0644); err != nil {
		t.Fatalf("failed to update README.md: %v", err)
	}
	runGit(t, cloneDir, "add", "README.md")
	runGit(t, cloneDir, "commit", "-m", "pr head commit")
	prHeadSHA := strings.TrimSpace(runGit(t, cloneDir, "rev-parse", "HEAD"))

	// Push the commit object straight into origin under refs/pull/<N>/head.
	// This mirrors how GitHub mirrors fork-PR head commits onto the base
	// repository: the contributor's branch name is never registered, but
	// the head ref is. Using `update-ref` on the bare repo directly would
	// fail because the object only lives in the clone's loose objects.
	runGit(t, cloneDir, "push", "origin", "HEAD:"+pullHeadRef(prNumber))

	// Clean up the helper branch and go back to main; the PR head must NOT
	// be reachable by a branch name on origin.
	runGit(t, cloneDir, "checkout", "main")
	runGit(t, cloneDir, "branch", "-D", "tmp-pr-head")

	// Sanity check: no remote branch matching the head ref exists. If this
	// assertion ever fails, the test would silently pass via the legacy
	// branch-name fetch and stop exercising the PR refspec path.
	out, _ := exec.Command("git", "-C", cloneDir, "ls-remote", "--heads", "origin", headBranchName).CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("test setup leaked branch %q on origin: %s", headBranchName, out)
	}

	return cloneDir, prHeadSHA
}

func pullHeadRef(prNumber int) string {
	return "refs/pull/" + itoa(prNumber) + "/head"
}

// itoa avoids pulling strconv into the test file just for one PR number.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
