package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/promptcontext"
)

func (h *MessageHandlers) taskPullRequestTargets(
	ctx context.Context,
	task *models.Task,
) []sysprompt.PullRequestTarget {
	return promptcontext.BuildPullRequestTargets(ctx, task.Repositories, func(resolveCtx context.Context, repositoryID string) (string, error) {
		repository, err := h.service.GetRepository(resolveCtx, repositoryID)
		if err != nil {
			return "", err
		}
		if repository == nil {
			return "", nil
		}
		return repository.Name, nil
	})
}
