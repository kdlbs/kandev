package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecreate_ManagedRefreshRequiresSelectedRemoteRef(t *testing.T) {
	for _, tt := range []struct {
		name           string
		checkoutBranch string
		prNumber       int
	}{
		{name: "selected branch", checkoutBranch: "feature/unpushed"},
		{name: "PR snapshot", prNumber: 42},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := initGitRepoWithRemote(t)
			runGit(t, repoPath, "branch", "feature/unpushed", "main")
			wantSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/unpushed"))
			worktreePath := filepath.Join(t.TempDir(), "task-selected", "repo-1")
			mgr := newRecreateTestManager(t)
			_, err := mgr.recreate(context.Background(), &Worktree{
				ID: "wt-selected", SessionID: "session-selected", TaskID: "task-selected",
				RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
				Branch: "feature/unpushed", Status: StatusDeleted,
			}, CreateRequest{
				SessionID: "session-selected", TaskID: "task-selected", RepositoryID: "repo-1",
				RepositoryPath: repoPath, BaseBranch: "main", CheckoutBranch: tt.checkoutBranch,
				PRNumber: tt.prNumber, RefreshRepository: func(context.Context) error {
					runGit(t, repoPath, "fetch", "origin")
					return nil
				},
			})
			if err == nil || !strings.Contains(err.Error(), "required fetched remote ref") {
				t.Fatalf("recreate() error = %v, want missing selected remote ref", err)
			}
			if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/unpushed")); got != wantSHA {
				t.Fatalf("local branch = %q, want preserved head %q", got, wantSHA)
			}
			if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
				t.Fatalf("failed recreation materialized a checkout: %v", err)
			}
		})
	}
}
