package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initGitRepoWithOriginAheadLocal builds the shape that matters here: a normal
// repository sitting on `main`, wired to a reachable bare `origin`, whose local
// `main` carries a commit that was never pushed. origin/main is therefore a
// strict ancestor of local main.
//
// Returns the working repository path and the local-only commit SHA.
func initGitRepoWithOriginAheadLocal(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	originPath := filepath.Join(base, "origin.git")
	repoPath := filepath.Join(base, "repo")

	runGitIn(t, base, "init", "--bare", "-b", "main", originPath)
	runGitIn(t, base, "init", "-b", "main", repoPath)
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")

	writeRepoFile(t, repoPath, "README.md", "initial\n")
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "origin", "main")

	// The commit that only exists locally — the equivalent of a user's unpushed
	// work on their base branch.
	writeRepoFile(t, repoPath, "local-only.txt", "unpushed\n")
	runGit(t, repoPath, "add", "local-only.txt")
	runGit(t, repoPath, "commit", "-m", "local-only commit")

	localHead := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))
	return repoPath, localHead
}

func writeRepoFile(t *testing.T, repoPath, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(contents), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// runGitIn runs git in an arbitrary directory (runGit requires the repo itself,
// which does not exist yet while we are creating it).
func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGit(t, dir, args...)
}

// TestPullBaseBranch_PullFailureKeepsLocalCommits pins the behaviour that a
// transient `git pull` failure must not silently rebase the worktree's starting
// point onto origin/<branch>.
//
// When the pull fails for an environmental reason (throttle contention, a
// timeout, a stale index.lock) origin/<branch> is not fresher than the local
// branch — here it is a strict ancestor of it. Returning the remote ref
// therefore discards the user's unpushed commits from the new worktree, and it
// does so silently: creation still succeeds, so nothing surfaces the loss.
//
// The two sibling fallbacks in manager_git.go (handleFetchFallback, and the
// non-current-branch path in resolveLocalBaseRef) already resolve to the local
// branch on failure. This one must agree with them.
func TestPullBaseBranch_PullFailureKeepsLocalCommits(t *testing.T) {
	repoPath, localHead := initGitRepoWithOriginAheadLocal(t)

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	// Let the fetch succeed (so we reach the pull) but force the pull to fail
	// the way contention makes it fail in a loaded run.
	mgr.pullTimeout = time.Nanosecond

	resolved := mgr.pullBaseBranch(context.Background(), repoPath, "main", nil)
	if resolved == "" {
		t.Fatal("pullBaseBranch returned an empty ref")
	}

	// Assert on the commit the ref actually points at, not on the ref's name:
	// the defect is that the worktree starts from the wrong commit, and a
	// name-only assertion would still pass if the mapping changed.
	resolvedSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", resolved))
	if resolvedSHA != localHead {
		t.Fatalf(
			"pullBaseBranch resolved %q to %s, which drops the unpushed local commit %s;\n"+
				"a failed pull must not move the worktree base backwards onto origin/main",
			resolved, resolvedSHA, localHead,
		)
	}
}

// TestPullBaseBranch_SuccessfulPullStillAdoptsRemoteCommits guards the happy
// path the fix must not regress: when the pull succeeds, the base branch is
// fast-forwarded and the worktree starts from the updated local branch, which
// now contains the remote commit.
func TestPullBaseBranch_SuccessfulPullStillAdoptsRemoteCommits(t *testing.T) {
	base := t.TempDir()
	originPath := filepath.Join(base, "origin.git")
	repoPath := filepath.Join(base, "repo")
	otherPath := filepath.Join(base, "other")

	runGitIn(t, base, "init", "--bare", "-b", "main", originPath)
	runGitIn(t, base, "init", "-b", "main", repoPath)
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, repoPath, "README.md", "initial\n")
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "remote", "add", "origin", originPath)
	runGit(t, repoPath, "push", "origin", "main")

	// A second clone pushes a commit the working repo has not seen yet.
	runGitIn(t, base, "clone", originPath, otherPath)
	runGit(t, otherPath, "config", "user.email", "other@example.com")
	runGit(t, otherPath, "config", "user.name", "Other User")
	runGit(t, otherPath, "config", "commit.gpgsign", "false")
	writeRepoFile(t, otherPath, "remote-only.txt", "pushed\n")
	runGit(t, otherPath, "add", "remote-only.txt")
	runGit(t, otherPath, "commit", "-m", "remote-only commit")
	runGit(t, otherPath, "push", "origin", "main")
	remoteHead := strings.TrimSpace(runGit(t, otherPath, "rev-parse", "HEAD"))

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	resolved := mgr.pullBaseBranch(context.Background(), repoPath, "main", nil)
	resolvedSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", resolved))
	if resolvedSHA != remoteHead {
		t.Fatalf(
			"pullBaseBranch resolved %q to %s, want the pulled remote commit %s",
			resolved, resolvedSHA, remoteHead,
		)
	}
}
