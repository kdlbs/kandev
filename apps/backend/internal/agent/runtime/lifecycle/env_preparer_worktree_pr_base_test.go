package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestWorktreePreparer_MultiRepoUnresolvedQualifiedBaseFailsDespiteValidSibling(t *testing.T) {
	isolatePreparerGitConfig(t)
	validRepo := initBareGitRepo(t, "valid-sibling")
	unresolvedRepo := initBareGitRepo(t, "unresolved-fork")
	upstreamBare := filepath.Join(t.TempDir(), "upstream.git")
	runGitForPreparerTest(t, t.TempDir(), "init", "--bare", "-b", "main", upstreamBare)
	runGitForPreparerTest(t, unresolvedRepo, "remote", "add", "upstream", upstreamBare)
	runGitForPreparerTest(t, unresolvedRepo, "push", "upstream", "main")
	runGitForPreparerTest(t, unresolvedRepo, "config",
		"url.file://"+upstreamBare+".insteadOf", "https://github.com/upstream/widget.git")
	preparer, manager := newPreparerForTest(t)
	base := testLifecycleForkPRBase()
	base.OID = strings.Repeat("f", 40)

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID: "task-mixed-fork", SessionID: "session-mixed-fork", TaskDirName: "task-mixed-fork_aaa",
		Repositories: []RepoPrepareSpec{
			{TaskRepositoryID: "task-repo-valid", RepositoryID: "repo-valid", RepositoryPath: validRepo,
				RepoName: "valid", BaseBranch: "main"},
			{TaskRepositoryID: "task-repo-fork", RepositoryID: "repo-fork", RepositoryPath: unresolvedRepo,
				RepoName: "fork", BaseBranch: base.Target.TargetBranch, CheckoutBranch: base.Target.HeadBranch,
				PRNumber: base.Target.Number, QualifiedPRBase: &base, RemoteSyncHandled: true},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Prepare returned a hard error instead of a failed result: %v", err)
	}
	if result.Success || result.Error == nil || !strings.Contains(result.Error.Error(), "OID changed") {
		t.Fatalf("mixed preparation = %#v, want required qualified-base OID failure", result)
	}
	active, err := manager.GetAllByTaskID(context.Background(), "task-mixed-fork")
	if err != nil {
		t.Fatalf("list task worktrees: %v", err)
	}
	for _, wt := range active {
		if wt.Status == worktree.StatusActive {
			t.Errorf("valid sibling hid unresolved required target; active worktree remains: %#v", wt)
		}
	}
}

func TestWorktreePreparer_CanceledQualifiedBaseDoesNotReturnPreparedWorkspace(t *testing.T) {
	isolatePreparerGitConfig(t)
	repoPath := initBareGitRepo(t, "canceled-fork-base")
	runGitForPreparerTest(t, repoPath, "config",
		"url.file://"+repoPath+".insteadOf", "https://github.com/upstream/widget.git")
	preparer, manager := newPreparerForTest(t)
	base := testLifecycleForkPRBase()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := preparer.Prepare(ctx, &EnvPrepareRequest{
		TaskID: "task-canceled-fork", SessionID: "session-canceled-fork", TaskDirName: "task-canceled-fork_aaa",
		RepositoryID: "repo-canceled-fork", RepositoryPath: repoPath, RepoName: "fork",
		BaseBranch: base.Target.TargetBranch, CheckoutBranch: base.Target.HeadBranch,
		PRNumber: base.Target.Number, QualifiedPRBase: &base, RemoteSyncHandled: true,
	}, nil)
	if err != nil {
		t.Fatalf("Prepare returned a hard error: %v", err)
	}
	if result.Success || !errors.Is(result.Error, context.Canceled) || len(result.Worktrees) != 0 {
		t.Fatalf("canceled preparation = %#v, want failed, unlaunched result with context.Canceled", result)
	}
	active, err := manager.GetAllByTaskID(context.Background(), "task-canceled-fork")
	if err != nil {
		t.Fatalf("list task worktrees: %v", err)
	}
	for _, wt := range active {
		if wt.Status == worktree.StatusActive {
			t.Errorf("canceled qualified base left an active worktree: %#v", wt)
		}
	}
}

func isolatePreparerGitConfig(t *testing.T) {
	t.Helper()
	globalConfig := filepath.Join(t.TempDir(), "empty-git-config")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		t.Fatalf("create isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func testLifecycleForkPRBase() models.PRBase {
	return models.PRBase{Target: models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature/fork", TargetBranch: "main",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork/widget", RemoteURL: "https://github.com/fork/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", RemoteURL: "https://github.com/upstream/widget.git",
		},
	}}
}
