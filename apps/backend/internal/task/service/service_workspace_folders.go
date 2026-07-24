package service

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) hydrateTaskWorkspaceFolders(ctx context.Context, task *models.Task) {
	if s.workspaceFolders == nil {
		return
	}
	folders, err := s.workspaceFolders.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task workspace folders", zap.Error(err))
		return
	}
	task.WorkspaceFolders = folders
}

func (s *Service) hydrateTaskWorkspaceFoldersBatch(ctx context.Context, tasks []*models.Task) {
	if len(tasks) == 0 || s.workspaceFolders == nil {
		return
	}
	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}
	folders, err := s.workspaceFolders.ListTaskWorkspaceFoldersByTaskIDs(ctx, taskIDs)
	if err != nil {
		s.logger.Error("failed to batch-load task workspace folders", zap.Error(err))
		return
	}
	for _, task := range tasks {
		task.WorkspaceFolders = folders[task.ID]
	}
}
