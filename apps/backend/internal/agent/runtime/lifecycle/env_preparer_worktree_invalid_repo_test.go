package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/worktree"
)

// TestWorktreePreparer_ValidateRepository_FailsOnNonGitPath guards against the
// production bug where a provider-backed repository row ends up with a
// non-empty LocalPath that points at a directory with no ".git" (e.g. a stale
// path from a moved/deleted clone, or a placeholder directory that was never
// actually cloned). Previously validateWorktreeRequest only checked that
// RepositoryPath was non-empty, so this bad path sailed through "Validate
// repository" and failed downstream inside the worktree manager with the
// less actionable "repository is not a git repository" error. It should now
// fail fast, at this step, with a clear message naming the offending path.
func TestWorktreePreparer_ValidateRepository_FailsOnNonGitPath(t *testing.T) {
	notAGitRepo := t.TempDir() // empty dir, no .git inside

	repos := map[string]*worktree.Repository{
		"repo-single": {ID: "repo-single"},
	}
	preparer, _, _ := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:         "task-bad-path",
		SessionID:      "sess-bad-path",
		TaskTitle:      "Bad Path Task",
		ExecutorType:   executor.NameStandalone,
		TaskDirName:    "bad-path_xxx",
		UseWorktree:    true,
		RepositoryID:   "repo-single",
		RepositoryPath: notAGitRepo,
		RepoName:       "single",
		BaseBranch:     "main",
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if res.Success {
		t.Fatal("expected prepare to fail for a repository path with no .git directory")
	}
	if !strings.Contains(res.ErrorMessage, "not a git repository") {
		t.Errorf("ErrorMessage = %q, want it to mention the path is not a git repository", res.ErrorMessage)
	}
	if !strings.Contains(res.ErrorMessage, notAGitRepo) {
		t.Errorf("ErrorMessage = %q, want it to name the offending path %q", res.ErrorMessage, notAGitRepo)
	}
}

// TestWorktreePreparer_MultiRepo_ValidateRepository_FailsOnNonGitPath is the
// regression guard for the Greptile review finding that the multi-repo path
// (prepareMultiRepo -> prepareOneRepo) didn't run the same non-git-path
// validation as the single-repo Prepare. Previously a stale secondary repo
// (RepositoryPath set but no ".git" inside) passed prepareOneRepo's
// validation and failed later, deep inside worktree.Manager.Create, with the
// less actionable ErrRepoNotGit. It should now fail fast during "Validate
// <repo> repository" with the same clear message the single-repo path uses.
func TestWorktreePreparer_MultiRepo_ValidateRepository_FailsOnNonGitPath(t *testing.T) {
	repoA := initBareGitRepo(t, "primary")
	staleSecondary := t.TempDir() // exists on disk, but has no .git inside

	preparer, _ := newPreparerForTest(t)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-bad-secondary",
		SessionID:    "sess-multi-bad-secondary",
		TaskTitle:    "Multi Repo Stale Secondary",
		ExecutorType: executor.NameStandalone,
		TaskDirName:  "multi-bad-secondary_eee",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-primary", RepositoryPath: repoA, RepoName: "primary", BaseBranch: "main"},
			{RepositoryID: "repo-secondary", RepositoryPath: staleSecondary, RepoName: "secondary", BaseBranch: "main"},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if res.Success {
		t.Fatal("expected prepare to fail when the secondary repo path has no .git directory")
	}
	if !strings.Contains(res.ErrorMessage, "not a git repository") {
		t.Errorf("ErrorMessage = %q, want it to mention the path is not a git repository", res.ErrorMessage)
	}
	if !strings.Contains(res.ErrorMessage, staleSecondary) {
		t.Errorf("ErrorMessage = %q, want it to name the offending path %q", res.ErrorMessage, staleSecondary)
	}
}
