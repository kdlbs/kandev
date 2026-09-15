package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) hydrateTaskWorkspaceFolders(ctx context.Context, task *models.Task) {
	store := s.workspaceFolders
	if store == nil {
		store = s.workspaceSourceStore()
	}
	if store == nil {
		return
	}
	folders, err := store.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task workspace folders", zap.Error(err))
		return
	}
	task.WorkspaceFolders = folders
}

func (s *Service) hydrateTaskWorkspaceFoldersBatch(ctx context.Context, tasks []*models.Task) {
	if len(tasks) == 0 {
		return
	}
	store := s.workspaceFolders
	if store == nil {
		store = s.workspaceSourceStore()
	}
	if store == nil {
		return
	}
	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}
	folders, err := store.ListTaskWorkspaceFoldersByTaskIDs(ctx, taskIDs)
	if err != nil {
		s.logger.Error("failed to batch-load task workspace folders", zap.Error(err))
		return
	}
	for _, task := range tasks {
		task.WorkspaceFolders = folders[task.ID]
	}
}
