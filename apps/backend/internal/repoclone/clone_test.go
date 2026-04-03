package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestClone_PreservesNonDefaultRemoteBranches(t *testing.T) {
	t.Parallel()

	originPath := initBareRepoWithReleaseBranch(t)
	targetPath := filepath.Join(t.TempDir(), "clone")

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	if err := cloner.clone(context.Background(), originPath, targetPath, "", ""); err != nil {
		t.Fatalf("clone() unexpected error: %v", err)
	}

	if !gitRefExists(t, targetPath, "refs/remotes/origin/release") {
		t.Fatal("expected cloned repo to contain origin/release for downstream worktree base branches")
	}
}

func initBareRepoWithReleaseBranch(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	originPath := filepath.Join(root, "origin.git")
	workPath := filepath.Join(root, "work")

	runGit(t, root, "init", "--bare", "-b", "main", originPath)
	runGit(t, root, "clone", originPath, workPath)
	runGit(t, workPath, "config", "user.email", "test@example.com")
	runGit(t, workPath, "config", "user.name", "Test User")

	readmePath := filepath.Join(workPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, workPath, "add", "README.md")
	runGit(t, workPath, "commit", "-m", "main commit")
	runGit(t, workPath, "push", "origin", "main")

	runGit(t, workPath, "checkout", "-b", "release")
	if err := os.WriteFile(readmePath, []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release README.md: %v", err)
	}
	runGit(t, workPath, "commit", "-am", "release commit")
	runGit(t, workPath, "push", "origin", "release")

	return originPath
}

func gitRefExists(t *testing.T, repoPath, ref string) bool {
	t.Helper()

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}
