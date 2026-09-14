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
	t.Setenv("GIT_AUTHOR_NAME", "Inherited Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "inherited-author@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Inherited Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "inherited-committer@example.com")

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

	t.Setenv("GIT_AUTHOR_NAME", "Another Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "another-author@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Another Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "another-committer@example.com")
	otherRepo := t.TempDir()
	runGit(t, otherRepo, "init", "-b", "main", ".")
	other, err := gitbootstrap.Ensure(context.Background(), otherRepo, "main")
	if err != nil {
		t.Fatalf("Ensure() with another inherited identity error = %v", err)
	}
	if other.Commit != first.Commit {
		t.Fatalf("baseline changed with inherited Git identity: first=%q other=%q", first.Commit, other.Commit)
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
