package process

import (
	"context"
	"strings"
	"testing"
)

func TestPrepareBaseBranchTarget(t *testing.T) {
	t.Run("origin remote keeps fetched target", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)

		operator := NewGitOperator(repoDir, newTestLogger(t), nil)
		_, target, err := operator.prepareBaseBranchTarget(context.Background(), "main")
		if err != nil {
			t.Fatalf("prepareBaseBranchTarget returned error: %v", err)
		}
		if target != "origin/main" {
			t.Fatalf("target = %q, want %q", target, "origin/main")
		}
	})

	t.Run("without origin uses local branch", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		runGit(t, repoDir, "remote", "remove", "origin")

		operator := NewGitOperator(repoDir, newTestLogger(t), nil)
		_, target, err := operator.prepareBaseBranchTarget(context.Background(), "main")
		if err != nil {
			t.Fatalf("prepareBaseBranchTarget returned error: %v", err)
		}
		if target != "refs/heads/main" {
			t.Fatalf("target = %q, want %q", target, "refs/heads/main")
		}
	})

	t.Run("origin fetch failure does not fall back locally", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		runGit(t, repoDir, "remote", "set-url", "origin", "/does/not/exist")

		operator := NewGitOperator(repoDir, newTestLogger(t), nil)
		_, target, err := operator.prepareBaseBranchTarget(context.Background(), "main")
		if target != "" {
			t.Errorf("target = %q, want empty target after fetch failure", target)
		}
		if err == nil || !strings.HasPrefix(err.Error(), "failed to fetch base branch:") {
			t.Fatalf("error = %v, want fetch error", err)
		}
	})

	t.Run("without origin reports missing local branch", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		runGit(t, repoDir, "remote", "remove", "origin")
		runGit(t, repoDir, "checkout", "-b", "feature/test")
		runGit(t, repoDir, "branch", "-D", "main")

		operator := NewGitOperator(repoDir, newTestLogger(t), nil)
		_, target, err := operator.prepareBaseBranchTarget(context.Background(), "main")
		if target != "" {
			t.Errorf("target = %q, want empty target", target)
		}
		if err == nil || !strings.HasPrefix(err.Error(), `base branch "main" does not exist locally`) {
			t.Fatalf("error = %v, want missing-local-base error", err)
		}
		if !strings.Contains(err.Error(), "fatal: Needed a single revision") {
			t.Fatalf("error = %v, want underlying git error", err)
		}
		if strings.Contains(err.Error(), "fetch") {
			t.Fatalf("missing local branch error unexpectedly fetched: %v", err)
		}
	})
}
