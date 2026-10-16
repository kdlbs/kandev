//go:build !windows

package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureCleanupHeadOIDs_MetadataRemovedDuringInspection(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	enclosing := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(enclosing, "other.txt"), []byte("other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, enclosing, "add", "other.txt")
	runGit(t, enclosing, "commit", "-m", "enclosing repository")
	checkout := filepath.Join(enclosing, "checkout")
	runGit(t, repo, "worktree", "add", checkout, "feature/pr-branch")
	want := strings.TrimSpace(runGit(t, checkout, "rev-parse", "HEAD"))
	if want == strings.TrimSpace(runGit(t, enclosing, "rev-parse", "HEAD")) {
		t.Fatal("test repositories must have distinct HEADs")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	scriptDir := writeFakeGitScript(t, `
if [ "${1:-}" = "rev-parse" ] && [ "${2:-}" = "--verify" ]; then
  rm -f .git
fi
exec "${KD_REAL_GIT:?}" "$@"
`)
	t.Setenv("KD_REAL_GIT", gitPath)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := newCaptureTestManager(t).CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
		ID: "removed-metadata", RepositoryPath: repo, Path: checkout, Branch: "feature/pr-branch",
	}})
	if err != nil {
		t.Fatalf("capture identity after metadata removal: %v", err)
	}
	if got["removed-metadata"] != want {
		t.Fatalf("identity = %q, want worktree branch %q", got["removed-metadata"], want)
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Fatalf("checkout did not survive metadata removal: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(checkout, ".git")); !os.IsNotExist(err) {
		t.Fatalf("metadata removal did not occur: %v", err)
	}
}
