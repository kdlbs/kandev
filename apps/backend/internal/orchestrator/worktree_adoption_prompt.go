package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type taskEnvironmentGuidanceReader interface {
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
}

// WorktreeAdoptionGuidance reports whether the first-turn context should tell
// the agent to report new Git worktrees. Only host Worktree and Local
// executors support adoption.
func (s *Service) WorktreeAdoptionGuidance(ctx context.Context, taskID, sessionID string) (bool, error) {
	reader, ok := s.repo.(taskEnvironmentGuidanceReader)
	if !ok {
		return false, nil
	}
	var environmentID string
	if sessionID != "" {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err == nil && session != nil {
			environmentID = session.TaskEnvironmentID
		}
	}
	if environmentID != "" {
		environment, err := reader.GetTaskEnvironment(ctx, environmentID)
		if err == nil && environment != nil {
			return supportsWorktreeAdoptionGuidance(environment.ExecutorType), nil
		}
	}
	if taskID == "" {
		return false, nil
	}
	environment, err := reader.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || environment == nil {
		return false, nil
	}
	return supportsWorktreeAdoptionGuidance(environment.ExecutorType), nil
}

func (s *Service) worktreeAdoptionGuidance(ctx context.Context, taskID, sessionID string) bool {
	enabled, err := s.WorktreeAdoptionGuidance(ctx, taskID, sessionID)
	return err == nil && enabled
}

func supportsWorktreeAdoptionGuidance(executorType string) bool {
	switch executorType {
	case string(models.ExecutorTypeWorktree), string(models.ExecutorTypeLocal), "local_pc":
		return true
	default:
		return false
	}
}
