package nomad

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPoolWorkspaceRemoveDetachesWorktreeAndPrunesBranch(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(t.TempDir(), "kandev-checkout")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	s := poolFenceState(t, root, "env-a")
	if err := os.MkdirAll(s.WorkspacePath, 0o700); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(s.WorkspacePath, "repo-main")
	gitIn(t, repo, "worktree", "add", "-q", "-b", PoolWorktreeBranch(s.EnvironmentID), wt, "main")
	command, err := WorkspaceRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("remove %d: %v %s", i, err, out)
		}
	}
	if _, err := os.Stat(s.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace remains: %v", err)
	}
	if out := gitIn(t, repo, "branch", "--list", PoolWorktreeBranch(s.EnvironmentID)); out != "" {
		t.Fatalf("per-environment branch remains: %q", out)
	}
	if out := gitIn(t, repo, "worktree", "list", "--porcelain"); strings.Contains(out, "env-a") {
		t.Fatalf("worktree registration remains: %s", out)
	}
	if gitIn(t, repo, "rev-parse", "--abbrev-ref", "HEAD") != "main" {
		t.Fatal("source checkout changed")
	}
}

func TestPoolWorkspaceRemoveRetainsWorkspaceWhenWorktreeCannotBeDetached(t *testing.T) {
	root := t.TempDir()
	s := poolFenceState(t, root, "env-a")
	wt := filepath.Join(s.WorkspacePath, "repo-main")
	if err := os.MkdirAll(wt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /nonexistent/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command, err := WorkspaceRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Command("sh", "-c", command).Run() == nil {
		t.Fatal("cleanup succeeded with an undetachable worktree")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("workspace was removed despite failed detach: %v", err)
	}
}

func TestPoolWorkspaceRemoveDeletesPerGenerationBranchesOnlyUnderThePoolPrefix(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(t.TempDir(), "kandev-checkout")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	s := poolFenceState(t, root, "env-a")
	if err := os.MkdirAll(s.WorkspacePath, 0o700); err != nil {
		t.Fatal(err)
	}
	generation := PoolWorktreeBranch(s.EnvironmentID) + "-g1"
	gitIn(t, repo, "worktree", "add", "-q", "-b", generation, filepath.Join(s.WorkspacePath, "repo-main"), "main")
	gitIn(t, repo, "branch", "feature/keep", "main")
	gitIn(t, repo, "worktree", "add", "-q", filepath.Join(s.WorkspacePath, "repo-user"), "feature/keep")
	command, err := WorkspaceRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("remove: %v %s", err, out)
	}
	if out := gitIn(t, repo, "branch", "--list", generation); out != "" {
		t.Fatalf("per-generation branch leaked: %q", out)
	}
	if out := gitIn(t, repo, "branch", "--list", "feature/keep"); out == "" {
		t.Fatal("a branch outside the pool prefix was deleted")
	}
}
