package worktree

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktreePRHeadCollisionPreservesLocalBranch(t *testing.T) {
	const branch = "feature/unit-7-reject-leg-wa-waj"

	repoPath, firstPRHeadSHA := initGitRepoWithPullRef(t, 3294, branch)
	runGit(t, repoPath, "fetch", "origin", pullHeadRef(3294)+":"+pullRequestSnapshotRef(3294))
	runGit(t, repoPath, "branch", branch, pullRequestSnapshotRef(3294))

	originURL := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin"))
	remoteClone := filepath.Join(t.TempDir(), "pr-3408-clone")
	runGit(t, filepath.Dir(remoteClone), "clone", originURL, remoteClone)
	runGit(t, remoteClone, "config", "user.email", "test@example.com")
	runGit(t, remoteClone, "config", "user.name", "Test User")
	runGit(t, remoteClone, "config", "commit.gpgsign", "false")
	runGit(t, remoteClone, "checkout", "-b", "tmp-pr-head-3408", "main")
	writeTestFile(t, filepath.Join(remoteClone, "pr-3408.txt"), "second PR head\n")
	runGit(t, remoteClone, "add", "pr-3408.txt")
	runGit(t, remoteClone, "commit", "-m", "second PR head")
	secondPRHeadSHA := strings.TrimSpace(runGit(t, remoteClone, "rev-parse", "HEAD"))
	runGit(t, remoteClone, "push", "origin", "HEAD:"+pullHeadRef(3408))
	runGit(t, remoteClone, "push", "origin", "main:refs/heads/"+branch)
	runGit(t, repoPath, "fetch", "origin", "refs/heads/"+branch+":refs/remotes/origin/"+branch)

	collisionBranch := branch + "-" + TaskDirSuffix("task-pr-3408")
	runGit(t, repoPath, "branch", collisionBranch, "main")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-pr-3408",
		SessionID:      "session-pr-3408",
		TaskTitle:      "PR 3408",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: branch,
		PRNumber:       3408,
		TaskDirName:    "task-pr-3408",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() should materialize the exact PR head: %v", err)
	}

	if wt.Branch != collisionBranch+"-1" {
		t.Fatalf("PR worktree should retry a colliding task branch, got %q, want %q", wt.Branch, collisionBranch+"-1")
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", branch)); got != firstPRHeadSHA {
		t.Fatalf("local branch SHA = %q, want preserved SHA %q", got, firstPRHeadSHA)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != secondPRHeadSHA {
		t.Fatalf("worktree HEAD SHA = %q, want exact PR 3408 head %q", got, secondPRHeadSHA)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", pullRequestSnapshotRef(3408))); got != secondPRHeadSHA {
		t.Fatalf("stored PR ref SHA = %q, want %q", got, secondPRHeadSHA)
	}
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}")
	cmd.Dir = wt.Path
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("PR worktree should not track a divergent source branch, got %q", strings.TrimSpace(string(out)))
	}
}
