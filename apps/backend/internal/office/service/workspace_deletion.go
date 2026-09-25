package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	workspaceDeletionPageSize     = 500
	workspaceDeletionPhaseTimeout = 60 * time.Second
)

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
	_, total, err := s.taskWorkspace.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, 1, "", true, true, false, false)
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

	cleanupBaseCtx := context.WithoutCancel(ctx)
	taskCancelCtx, cancelTaskCancel := workspaceDeletionPhaseContext(cleanupBaseCtx)
	s.cancelWorkspaceTasks(taskCancelCtx, tasks)
	cancelTaskCancel()

	runCleanupCtx, cancelRunCleanup := workspaceDeletionPhaseContext(cleanupBaseCtx)
	if err := s.stopWorkspaceRunSessions(runCleanupCtx, workspaceID); err != nil {
		cancelRunCleanup()
		return err
	}
	cancelRunCleanup()

	if s.configSyncCleaner != nil {
		configSyncCleanupCtx, cancelConfigSyncCleanup := workspaceDeletionPhaseContext(cleanupBaseCtx)
		unlock, err := s.configSyncCleaner.PurgeForWorkspaceDeletion(configSyncCleanupCtx, workspaceID)
		cancelConfigSyncCleanup()
		if err != nil {
			return fmt.Errorf("clean config sync data: %w", err)
		}
		// Held until every remaining teardown step below has run, so a config
		// sync run queued behind this lock cannot write rows back in after
		// DeleteWorkspaceData (which never touches config sync's own tables)
		// has already deleted the workspace's other data.
		defer unlock()
	}
	if s.workspaceGroupCleaner != nil {
		groupCleanupCtx, cancelGroupCleanup := workspaceDeletionPhaseContext(cleanupBaseCtx)
		if err := s.workspaceGroupCleaner.CleanupWorkspaceGroups(groupCleanupCtx, workspaceID); err != nil {
			cancelGroupCleanup()
			return fmt.Errorf("clean workspace groups: %w", err)
		}
		cancelGroupCleanup()
	}
	dataDeleteCtx, cancelDataDelete := workspaceDeletionPhaseContext(cleanupBaseCtx)
	defer cancelDataDelete()
	if err := s.deleteWorkspaceTasks(dataDeleteCtx, tasks); err != nil {
		return err
	}
	if err := s.repo.DeleteWorkspaceData(dataDeleteCtx, workspaceID); err != nil {
		return fmt.Errorf("delete office workspace data: %w", err)
	}
	if err := s.taskWorkspace.DeleteWorkspace(dataDeleteCtx, workspaceID); err != nil {
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

// stopWorkspaceRunSessions terminates every run-owned process while its
// durable session identity is still available. Any stop or persistence error
// aborts deletion so a live process cannot outlive the rows that identify it.
func (s *Service) stopWorkspaceRunSessions(ctx context.Context, workspaceID string) error {
	sessions, err := s.repo.ListLiveRunSessionsForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list live run sessions for deletion: %w", err)
	}
	if len(sessions) == 0 {
		return nil
	}
	if s.runStopper == nil {
		return errors.New("stop run sessions for deletion: run execution stopper not configured")
	}
	var firstErr error
	for _, session := range sessions {
		if _, requestErr := s.repo.RequestRunSessionCancellation(ctx, session.ID); requestErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("request cancellation for run session %s: %w", session.ID, requestErr)
			}
			continue
		}
		stopErr := s.runStopper.Stop(ctx, session.ExecutionID, "workspace deleted")
		switch {
		case stopErr == nil:
			if _, finishErr := s.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateCancelled, "workspace deleted"); finishErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("finish cancelled run session %s: %w", session.ID, finishErr)
			}
		case errors.Is(stopErr, runtimeapi.ErrNotFound):
			if _, finishErr := s.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateInterrupted, "runtime execution not found"); finishErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("finish missing run session %s: %w", session.ID, finishErr)
			}
		default:
			if firstErr == nil {
				firstErr = fmt.Errorf("stop run session %s: %w", session.ID, stopErr)
			}
		}
	}
	return firstErr
}

func (s *Service) deleteWorkspaceTasks(ctx context.Context, tasks []*taskmodels.Task) error {
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		if s.taskTreeDeleter != nil {
			if err := s.taskTreeDeleter(ctx, task.ID); err != nil {
				return fmt.Errorf("delete task %s through lifecycle: %w", task.ID, err)
			}
			continue
		}
		if err := s.taskWorkspace.DeleteTask(ctx, task.ID); err != nil {
			return fmt.Errorf("delete task %s: %w", task.ID, err)
		}
	}
	return nil
}

func workspaceDeletionPhaseContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, workspaceDeletionPhaseTimeout)
}

func (s *Service) listAllWorkspaceTasks(ctx context.Context, workspaceID string) ([]*taskmodels.Task, error) {
	var all []*taskmodels.Task
	for page := 1; ; page++ {
		tasks, total, err := s.taskWorkspace.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", page, workspaceDeletionPageSize, "", true, true, false, false,
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
