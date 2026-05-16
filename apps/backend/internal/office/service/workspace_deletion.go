package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const workspaceDeletionPageSize = 500

// WorkspaceDeletionSummary describes the permanent-delete impact for a workspace.
type WorkspaceDeletionSummary struct {
	WorkspaceName string `json:"workspace_name"`
	Tasks         int    `json:"tasks"`
	Agents        int    `json:"agents"`
	Skills        int    `json:"skills"`
	ConfigPath    string `json:"config_path"`
}

// GetWorkspaceDeletionSummary returns counts and filesystem path for confirmation UI.
func (s *Service) GetWorkspaceDeletionSummary(ctx context.Context, workspaceID string) (*WorkspaceDeletionSummary, error) {
	if s.taskWorkspace == nil {
		return nil, fmt.Errorf("task workspace service not configured")
	}
	workspace, err := s.taskWorkspace.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	_, total, err := s.taskWorkspace.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, 1, true, true, false, false)
	if err != nil {
		return nil, fmt.Errorf("count tasks: %w", err)
	}
	counts, err := s.repo.GetWorkspaceDeletionCounts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &WorkspaceDeletionSummary{
		WorkspaceName: workspace.Name,
		Tasks:         total,
		Agents:        counts.Agents,
		Skills:        counts.Skills,
		ConfigPath:    s.workspaceConfigPath(workspace.Name),
	}, nil
}

// DeleteWorkspace permanently deletes a workspace and its office/task/config data.
func (s *Service) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	if s.taskWorkspace == nil {
		return fmt.Errorf("task workspace service not configured")
	}
	workspace, err := s.taskWorkspace.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	tasks, err := s.listAllWorkspaceTasks(ctx, workspaceID)
	if err != nil {
		return err
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	s.cancelWorkspaceTasks(cleanupCtx, tasks)

	if err := s.repo.DeleteWorkspaceData(ctx, workspaceID); err != nil {
		return fmt.Errorf("delete office workspace data: %w", err)
	}
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		if err := s.taskWorkspace.DeleteTask(cleanupCtx, task.ID); err != nil {
			return fmt.Errorf("delete task %s: %w", task.ID, err)
		}
	}
	if err := s.taskWorkspace.DeleteWorkspace(cleanupCtx, workspaceID); err != nil {
		return fmt.Errorf("delete workspace row: %w", err)
	}
	if s.cfgWriter != nil {
		if err := s.cfgWriter.DeleteWorkspace(workspace.Name); err != nil {
			return fmt.Errorf("delete workspace config: %w", err)
		}
	}
	s.logger.Info("workspace permanently deleted",
		zap.String("workspace_id", workspaceID),
		zap.String("workspace_name", workspace.Name),
		zap.Int("tasks", len(tasks)))
	return nil
}

func (s *Service) listAllWorkspaceTasks(ctx context.Context, workspaceID string) ([]*taskmodels.Task, error) {
	var all []*taskmodels.Task
	for page := 1; ; page++ {
		tasks, total, err := s.taskWorkspace.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", page, workspaceDeletionPageSize, true, true, false, false,
		)
		if err != nil {
			return nil, fmt.Errorf("list workspace tasks: %w", err)
		}
		all = append(all, tasks...)
		if len(all) >= total || len(tasks) == 0 {
			return all, nil
		}
	}
}

func (s *Service) cancelWorkspaceTasks(ctx context.Context, tasks []*taskmodels.Task) {
	if s.taskCanceller == nil {
		return
	}
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		if err := s.taskCanceller.CancelTaskExecution(ctx, task.ID, "workspace deleted", true); err != nil {
			s.logger.Warn("failed to cancel task during workspace deletion",
				zap.String("task_id", task.ID),
				zap.Error(err))
		}
	}
}

func (s *Service) workspaceConfigPath(workspaceName string) string {
	if s.cfgWriter != nil {
		return s.cfgWriter.WorkspacePath(workspaceName)
	}
	if s.cfgLoader != nil {
		return filepath.Join(s.cfgLoader.BasePath(), "workspaces", workspaceName)
	}
	return ""
}
