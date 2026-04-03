package process

import (
	"context"
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
