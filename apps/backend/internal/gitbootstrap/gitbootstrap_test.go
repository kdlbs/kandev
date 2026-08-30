package gitbootstrap_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitbootstrap"
)

func TestEnsureCreatesDeterministicEmptyBaseline(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main", ".")

	first, err := gitbootstrap.Ensure(context.Background(), repoPath, "main")
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if first.Commit == "" {
		t.Fatal("Ensure() returned an empty baseline commit")
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/heads/main")); got != first.Commit {
		t.Fatalf("base branch = %q, want %q", got, first.Commit)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", first.MarkerRef)); got != first.Commit {
		t.Fatalf("marker = %q, want %q", got, first.Commit)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", first.Commit+"^{tree}")); got != gitbootstrap.EmptyTreeSHA {
		t.Fatalf("baseline tree = %q, want empty tree %q", got, gitbootstrap.EmptyTreeSHA)
	}
	entries, err := filepath.Glob(filepath.Join(repoPath, "*"))
	if err != nil {
		t.Fatalf("list repository files: %v", err)
	}
	for _, entry := range entries {
		if filepath.Base(entry) != ".git" {
			t.Fatalf("baseline created working-tree files: %v", entries)
		}
	}

	second, err := gitbootstrap.Ensure(context.Background(), repoPath, "main")
	if err != nil {
		t.Fatalf("second Ensure() error = %v", err)
	}
	if second.Commit != first.Commit {
		t.Fatalf("second baseline commit = %q, want deterministic %q", second.Commit, first.Commit)
	}
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
