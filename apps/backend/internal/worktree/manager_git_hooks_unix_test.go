//go:build unix

package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateRunsRepositoryHooksOnlyWhenOptedIn proves the repository's own
// hook executes during worktree materialization only when the request opts in.
func TestCreateRunsRepositoryHooksOnlyWhenOptedIn(t *testing.T) {
	for _, tc := range []struct {
		name      string
		runHooks  bool
		wantMarks bool
	}{{"default isolates hooks", false, false}, {"opt-in runs hooks", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}
			repoPath := initGitRepoForWorktreeTest(t)
			marker := filepath.Join(t.TempDir(), "hook-ran")
			hook := "#!/bin/sh\n: > '" + marker + "'\n"
			if err := os.WriteFile(filepath.Join(repoPath, ".git", "hooks", "post-checkout"), []byte(hook), 0o700); err != nil {
				t.Fatalf("write hook: %v", err)
			}

			if _, err := mgr.Create(context.Background(), CreateRequest{
				TaskID: "task-1", SessionID: "session-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
				BaseBranch: "main", TaskDirName: "task-1", RepoName: "repo-1", RunGitHooks: tc.runHooks,
			}); err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			_, statErr := os.Stat(marker)
			if ran := statErr == nil; ran != tc.wantMarks {
				t.Fatalf("hook ran = %t, want %t", ran, tc.wantMarks)
			}
		})
	}
}
