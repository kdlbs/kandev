package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitOperatorPush_PreservesExistingUpstream(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)

	runGit(t, repoDir, "checkout", "-b", "feature/pr-branch")
	writeFile(t, repoDir, "feature.txt", "feature branch\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feature commit")
	runGit(t, repoDir, "push", "-u", "origin", "feature/pr-branch")

	runGit(t, repoDir, "checkout", "main")
	worktreeBase := t.TempDir()
	directDir := filepath.Join(worktreeBase, "wt-direct")
	suffixedDir := filepath.Join(worktreeBase, "wt-sfx")
	runGit(t, repoDir, "worktree", "add", directDir, "feature/pr-branch")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/pr-branch-sfx", suffixedDir, "origin/feature/pr-branch")

	runGit(t, suffixedDir, "branch", "--set-upstream-to=origin/feature/pr-branch", "feature/pr-branch-sfx")
	writeFile(t, suffixedDir, "feature.txt", "feature branch\nlocal change\n")
	runGit(t, suffixedDir, "add", ".")
	runGit(t, suffixedDir, "commit", "-m", "local change")

	before := strings.TrimSpace(runGit(t, suffixedDir, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if before != "origin/feature/pr-branch" {
		t.Fatalf("upstream before push = %q, want %q", before, "origin/feature/pr-branch")
	}

	gitOp := NewGitOperator(suffixedDir, log, nil)
	result, err := gitOp.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Push failed: %+v", result)
	}

	after := strings.TrimSpace(runGit(t, suffixedDir, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if after != "origin/feature/pr-branch" {
		t.Fatalf("upstream after push = %q, want %q", after, "origin/feature/pr-branch")
	}
}

func TestParseCommitDiff_PathsWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)

	// Create a file with spaces in its path, commit it, then verify parseCommitDiff
	// extracts the correct unquoted file path.
	dir := filepath.Join(repoDir, "path with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "file.md", "hello world\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add spaced path")
	sha := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.ShowCommit(context.Background(), sha)
	if err != nil {
		t.Fatalf("ShowCommit error: %v", err)
	}
	if !result.Success {
		t.Fatalf("ShowCommit failed: %+v", result)
	}

	expectedPath := "path with spaces/file.md"
	if _, exists := result.Files[expectedPath]; !exists {
		keys := make([]string, 0, len(result.Files))
		for k := range result.Files {
			keys = append(keys, k)
		}
		t.Errorf("expected Files to contain key %q, got keys: %v", expectedPath, keys)
	}
}
