package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type kanbanWorkspaceLister interface {
	ListKanbanTasksByWorkspace(context.Context, string, models.KanbanTaskQuery) ([]*models.Task, int, error)
}

func (s *Service) ListKanbanTasksByWorkspace(ctx context.Context, workspaceID string, query models.KanbanTaskQuery) ([]*models.Task, int, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, 0, err
	}
	lister, ok := s.tasks.(kanbanWorkspaceLister)
	if !ok {
		return nil, 0, fmt.Errorf("kanban workspace listing unavailable")
	}
	tasks, total, err := lister.ListKanbanTasksByWorkspace(ctx, workspaceID, query)
	if err != nil {
		return nil, 0, err
	}
	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	s.hydrateTaskWorkspaceFoldersBatch(ctx, tasks)
	return tasks, total, nil
}
