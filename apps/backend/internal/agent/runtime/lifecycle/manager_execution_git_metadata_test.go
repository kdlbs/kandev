package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceExecutionGitMetadataProjectionsRejectsMissingCurrentRepository(t *testing.T) {
	repositoryA := initBareGitRepo(t, "workspace-current-a")
	repositoryB := initBareGitRepo(t, "workspace-current-b")
	preparer, worktreeManager := newPreparerForTest(t)
	prepared, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID: "task-current-repositories", SessionID: "session-current-repositories",
		ExecutorType: executor.NameStandalone, TaskDirName: "current-repositories",
		RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main",
		Repositories: []RepoPrepareSpec{{
			RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main",
		}},
	}, nil)
	if err != nil || !prepared.Success {
		t.Fatalf("prepare worktree: result=%+v err=%v", prepared, err)
	}

	manager, _ := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{})
	manager.SetWorktreeManager(worktreeManager)
	_, err = manager.workspaceExecutionGitMetadataProjections(context.Background(), "task-current-repositories", &WorkspaceInfo{
		ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{
			{RepositoryID: "repo-a", RepositoryPath: repositoryA},
			{RepositoryID: "repo-b", RepositoryPath: repositoryB},
		},
	})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionInvalid) {
		t.Fatalf("workspaceExecutionGitMetadataProjections() error = %v, want missing repository rejection", err)
	}
}

func TestWorkspaceExecutionGitMetadataProjectionsIgnoresStaleRepositoryRecord(t *testing.T) {
	repositoryA := initBareGitRepo(t, "workspace-current-a")
	repositoryB := initBareGitRepo(t, "workspace-current-b")
	preparer, worktreeManager := newPreparerForTest(t)
	prepared, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID: "task-stale-repository", SessionID: "session-stale-repository",
		ExecutorType: executor.NameStandalone, TaskDirName: "stale-repository",
		RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main"},
			{RepositoryID: "repo-b", RepositoryPath: repositoryB, RepoName: "repo-b", BaseBranch: "main"},
		},
	}, nil)
	if err != nil || !prepared.Success {
		t.Fatalf("prepare worktrees: result=%+v err=%v", prepared, err)
	}

	manager, _ := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{})
	manager.SetWorktreeManager(worktreeManager)
	projections, err := manager.workspaceExecutionGitMetadataProjections(context.Background(), "task-stale-repository", &WorkspaceInfo{
		ExecutorType:          string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{{RepositoryID: "repo-a", RepositoryPath: repositoryA}},
	})
	if err != nil {
		t.Fatalf("workspaceExecutionGitMetadataProjections() error = %v", err)
	}
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want one current repository projection", len(projections))
	}
}

func TestWorkspaceExecutionGitMetadataProjectionsRejectsMalformedCurrentRecord(t *testing.T) {
	repositoryA := initBareGitRepo(t, "workspace-malformed-a")
	repositoryB := initBareGitRepo(t, "workspace-malformed-b")
	preparer, worktreeManager, store := newPreparerForTestWithStore(t)
	prepared, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID: "task-malformed-repository", SessionID: "session-malformed-repository",
		ExecutorType: executor.NameStandalone, TaskDirName: "malformed-repository",
		RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-a", RepositoryPath: repositoryA, RepoName: "repo-a", BaseBranch: "main"},
			{RepositoryID: "repo-b", RepositoryPath: repositoryB, RepoName: "repo-b", BaseBranch: "main"},
		},
	}, nil)
	if err != nil || !prepared.Success {
		t.Fatalf("prepare worktrees: result=%+v err=%v", prepared, err)
	}
	store.mu.Lock()
	for _, wt := range store.worktrees {
		if wt.RepositoryID == "repo-b" {
			wt.Path = ""
		}
	}
	store.mu.Unlock()

	manager, _ := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{})
	manager.SetWorktreeManager(worktreeManager)
	_, err = manager.workspaceExecutionGitMetadataProjections(context.Background(), "task-malformed-repository", &WorkspaceInfo{
		ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{
			{RepositoryID: "repo-a", RepositoryPath: repositoryA},
			{RepositoryID: "repo-b", RepositoryPath: repositoryB},
		},
	})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionInvalid) {
		t.Fatalf("workspaceExecutionGitMetadataProjections() error = %v, want malformed record rejection", err)
	}
}
