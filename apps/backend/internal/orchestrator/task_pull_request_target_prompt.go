package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/promptcontext"
	"go.uber.org/zap"
)

type taskPullRequestTargetStore interface {
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
}

func (s *Service) taskPullRequestTargets(ctx context.Context, taskID string) []sysprompt.PullRequestTarget {
	if s.promptTargets == nil {
		return nil
	}
	links, err := s.promptTargets.ListTaskRepositories(ctx, taskID)
	if err != nil {
		s.logPullRequestTargetLookupFailure("failed to load task repositories", taskID, "", err)
		return nil
	}
	return promptcontext.BuildPullRequestTargets(ctx, links, func(resolveCtx context.Context, repositoryID string) (string, error) {
		repository, err := s.promptTargets.GetRepository(resolveCtx, repositoryID)
		if err != nil {
			s.logPullRequestTargetLookupFailure(
				"failed to load repository for pull request target prompt", taskID, repositoryID, err,
			)
			return "", err
		}
		if repository == nil {
			return "", nil
		}
		return repository.Name, nil
	})
}

func (s *Service) logPullRequestTargetLookupFailure(message, taskID, repositoryID string, err error) {
	if s.logger == nil {
		return
	}
	fields := []zap.Field{zap.String("task_id", taskID), zap.Error(err)}
	if repositoryID != "" {
		fields = append(fields, zap.String("repository_id", repositoryID))
	}
	s.logger.Warn(message, fields...)
}

func (s *Service) addTaskPullRequestTargetContext(
	ctx context.Context,
	taskID, prompt string,
	passthrough bool,
) (string, string) {
	targets := s.taskPullRequestTargets(ctx, taskID)
	if passthrough {
		return sysprompt.PrependPullRequestTargetInstruction(prompt, targets), ""
	}
	return sysprompt.InjectPullRequestTargetContext(prompt, targets)
}
