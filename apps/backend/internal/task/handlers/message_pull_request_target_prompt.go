package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func (h *MessageHandlers) taskPullRequestTargets(
	ctx context.Context,
	task *models.Task,
) []sysprompt.PullRequestTarget {
	targets := make([]sysprompt.PullRequestTarget, 0, len(task.Repositories))
	for _, link := range task.Repositories {
		if link.BranchPolicyID == "" || link.BranchPolicyPullRequestTarget == "" {
			continue
		}
		repositoryName := link.RepositoryID
		if repository, err := h.service.GetRepository(ctx, link.RepositoryID); err == nil && repository != nil && repository.Name != "" {
			repositoryName = repository.Name
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
