package promptcontext

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuildPullRequestTargets(t *testing.T) {
	links := []*models.TaskRepository{
		{RepositoryID: "repo-1", BaseBranch: "develop", CheckoutBranch: "feature/task", BranchPolicyID: "policy-1", BranchPolicyPullRequestTarget: "main"},
		{RepositoryID: "repo-2", BaseBranch: "main", BranchPolicyID: "policy-2", BranchPolicyPullRequestTarget: "release"},
		{RepositoryID: "repo-3", BranchPolicyID: "policy-3"},
		{RepositoryID: "repo-4", BranchPolicyPullRequestTarget: "main"},
		nil,
	}

	targets := BuildPullRequestTargets(context.Background(), links, func(_ context.Context, repositoryID string) (string, error) {
		if repositoryID == "repo-2" {
			return "", errors.New("repository unavailable")
		}
		return "repo-1-name", nil
	})

	require.Equal(t, []sysprompt.PullRequestTarget{
		{RepositoryName: "repo-1-name", WorkingBranch: "feature/task", TargetBranch: "main"},
		{RepositoryName: "repo-2", WorkingBranch: "main", TargetBranch: "release"},
	}, targets)
}
