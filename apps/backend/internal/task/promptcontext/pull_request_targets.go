package promptcontext

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// RepositoryNameResolver resolves a repository's display name for prompt context.
// An error leaves the repository ID as the stable fallback name.
type RepositoryNameResolver func(context.Context, string) (string, error)

// BuildPullRequestTargets converts task repository snapshots into the prompt
// context used by both task orchestration and task message handlers.
func BuildPullRequestTargets(
	ctx context.Context,
	links []*models.TaskRepository,
	resolveRepositoryName RepositoryNameResolver,
) []sysprompt.PullRequestTarget {
	targets := make([]sysprompt.PullRequestTarget, 0, len(links))
	for _, link := range links {
		if link == nil || link.BranchPolicyID == "" || link.BranchPolicyPullRequestTarget == "" {
			continue
		}
		repositoryName := link.RepositoryID
		if resolveRepositoryName != nil {
			if name, err := resolveRepositoryName(ctx, link.RepositoryID); err == nil && name != "" {
				repositoryName = name
			}
		}
		workingBranch := link.CheckoutBranch
		if workingBranch == "" {
			workingBranch = link.BaseBranch
		}
		targets = append(targets, sysprompt.PullRequestTarget{
			RepositoryName: repositoryName,
			WorkingBranch:  workingBranch,
			TargetBranch:   link.BranchPolicyPullRequestTarget,
		})
	}
	return targets
}
