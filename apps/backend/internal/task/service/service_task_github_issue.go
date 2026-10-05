package service

import (
	"context"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// UpdateTaskGitHubIssue changes only the metadata-backed GitHub issue link.
func (s *Service) UpdateTaskGitHubIssue(ctx context.Context, id string, link *models.TaskGitHubIssueLink) (*models.Task, error) {
	if err := s.authorizeTaskScope(ctx, id, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, id); err != nil {
		return nil, err
	}
	task, err := s.tasks.UpdateTaskGitHubIssue(ctx, id, link)
	if err != nil {
		return nil, err
	}
	task = s.reloadTaskAfterMutation(ctx, id, task, "update GitHub issue")
	repos, err := s.taskRepos.ListTaskRepositories(ctx, id)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}
	s.PublishTaskUpdated(ctx, task)
	return task, nil
}
