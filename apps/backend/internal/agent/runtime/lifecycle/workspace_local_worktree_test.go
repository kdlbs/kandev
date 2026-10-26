package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func localWorktreeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestProvisionLocalPoolWorktreesCreatesWorktreeAndKeepsRemoteRepos(t *testing.T) {
	repo := t.TempDir()
	localWorktreeGit(t, repo, "init", "-q", "-b", "main")
	localWorktreeGit(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	workspace := t.TempDir()
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/kandev.git", BaseBranch: "main"},
		{RepoName: "other", RepositoryURL: "https://example.test/other.git", BaseBranch: "main"},
	}}
	projection, err := remoteWorkspaceProjectionFromLaunch(req, true)
	if err != nil || len(projection) != 2 {
		t.Fatalf("projection: %v %d", err, len(projection))
	}
	instance := &ExecutorInstance{WorkspacePath: workspace, LocalWorktreeBranch: "kandev/pool/env-a"}
	remaining, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || !strings.HasPrefix(remaining[0].Destination, "other-") {
		t.Fatalf("remaining projection = %+v", remaining)
	}
	wt := filepath.Join(workspace, projection[0].Destination)
	if got := localWorktreeGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != "kandev/pool/env-a" {
		t.Fatalf("worktree branch = %q", got)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	again, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection)
	if err != nil || len(again) != 1 {
		t.Fatalf("second call must reuse the worktree: %v %d", err, len(again))
	}
}

func TestProvisionLocalPoolWorktreesIsNoOpOffLocalNodes(t *testing.T) {
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: "/x", RepositoryURL: "https://example.test/k.git", BaseBranch: "main"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	got, err := provisionLocalPoolWorktrees(context.Background(), req, &ExecutorInstance{WorkspacePath: t.TempDir()}, projection)
	if err != nil || len(got) != 1 {
		t.Fatalf("remote node must keep materialization: %v %d", err, len(got))
	}
}

func TestProvisionLocalPoolWorktreesFailsClosedOnMissingBaseBranch(t *testing.T) {
	repo := t.TempDir()
	localWorktreeGit(t, repo, "init", "-q", "-b", "main")
	localWorktreeGit(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "nope"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	if _, err := provisionLocalPoolWorktrees(context.Background(), req, &ExecutorInstance{WorkspacePath: t.TempDir(), LocalWorktreeBranch: "kandev/pool/e"}, projection); err == nil {
		t.Fatal("expected an error for an unknown base branch")
	}
}
