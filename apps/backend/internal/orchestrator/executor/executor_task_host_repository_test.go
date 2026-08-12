package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestEnvironmentReposForLaunchPersistsDockerPhysicalRepositoryOrder(t *testing.T) {
	repos := environmentReposForLaunch(&LaunchAgentRequest{
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		Repositories: []RepoSpec{
			{RepositoryID: "repo-alpha", BranchIdentitySlug: "main", Position: 20},
			{RepositoryID: "repo-beta", BranchIdentitySlug: "release", Position: 10},
		},
	}, &LaunchAgentResponse{})

	if len(repos) != 2 {
		t.Fatalf("docker environment repository count = %d, want 2", len(repos))
	}
	for index, want := range []struct {
		repositoryID string
		branchSlug   string
	}{
		{repositoryID: "repo-alpha", branchSlug: "main"},
		{repositoryID: "repo-beta", branchSlug: "release"},
	} {
		got := repos[index]
		if got.RepositoryID != want.repositoryID || got.BranchSlug != want.branchSlug || got.Position != index {
			t.Fatalf("docker environment repository %d = %#v, want id=%q branch=%q physical_position=%d",
				index, got, want.repositoryID, want.branchSlug, index)
		}
		if got.WorktreeID != "" || got.WorktreePath != "" {
			t.Fatalf("docker physical mapping invented a host worktree: %#v", got)
		}
	}
}

func TestEnvironmentReposForLaunchPersistsSingleDockerRepository(t *testing.T) {
	repos := environmentReposForLaunch(&LaunchAgentRequest{
		ExecutorType:       string(models.ExecutorTypeLocalDocker),
		RepositoryID:       "repo-alpha",
		BranchIdentitySlug: "main",
	}, &LaunchAgentResponse{})

	if len(repos) != 1 || repos[0].RepositoryID != "repo-alpha" || repos[0].BranchSlug != "main" || repos[0].Position != 0 {
		t.Fatalf("single Docker repository mapping = %#v", repos)
	}
}
