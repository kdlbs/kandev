package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// ListCoordinatorOriginTasks returns every task with origin "coordinator"
// (docs/specs/coordinator/system-design/copilot.md#conversation-cleanup),
// for the coordinator package's conversation cleanup pass and coordinator
// deletion. It carries no workspace scope of its own: its callers are the
// coordinator deletion route (already workspace.manage authorized before it
// reaches this call) and the startup cleanup pass (an internal, unscoped
// caller).
func (s *Service) ListCoordinatorOriginTasks(ctx context.Context, workspaceID string) ([]*models.Task, error) {
	return s.tasks.ListCoordinatorOriginTasks(ctx, workspaceID)
}
