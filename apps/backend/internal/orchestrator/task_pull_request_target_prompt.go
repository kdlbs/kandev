package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
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
	targets := make([]sysprompt.PullRequestTarget, 0, len(links))
	for _, link := range links {
		if link.BranchPolicyID == "" || link.BranchPolicyPullRequestTarget == "" {
			continue
		}
		repositoryName := link.RepositoryID
		repository, repoErr := s.promptTargets.GetRepository(ctx, link.RepositoryID)
		if repoErr != nil {
			s.logPullRequestTargetLookupFailure(
				"failed to load repository for pull request target prompt", taskID, link.RepositoryID, repoErr,
			)
		} else if repository != nil && repository.Name != "" {
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
