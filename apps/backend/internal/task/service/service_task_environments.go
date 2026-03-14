package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// GetTaskEnvironmentByTaskID returns the active task environment for a task.
// Returns nil if no environment exists yet.
func (s *Service) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	return s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
}
