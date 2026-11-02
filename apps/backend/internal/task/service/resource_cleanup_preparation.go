package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) capturePreparedTaskCleanup(ctx context.Context, job *models.TaskResourceCleanupJob, deleteEnvironmentRow bool, options DeleteTaskOptions) error {
	taskID := job.TaskID
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task for cleanup source manifest: %w", err)
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task sessions for cleanup snapshot: %w", err)
	}
	stopTargets, err := s.buildStopTargets(ctx, taskID, sessions)
	if err != nil {
		return fmt.Errorf("list runtime cleanup inventory: %w", err)
	}
	worktrees, err := s.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list worktrees for cleanup snapshot: %w", err)
	}
	worktreeHeadOIDs, preparationErrors, err := s.captureCleanupHeadOIDsForMode(ctx, worktrees, options)
	if err != nil {
		return err
	}
	worktreeTaskDirNames := captureWorktreeTaskDirNames(worktrees)
	worktreeBranchMetadata := captureWorktreeBranchMetadata(worktrees)
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	if err != nil {
		return fmt.Errorf("lookup task environment for cleanup: %w", err)
	}
	sshTaskDirs, err := s.gatherSSHReclaimTargets(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list remote task directories for cleanup snapshot: %w", err)
	}
	snapshot := taskResourceCleanupSnapshot{
		AllowUnverifiedWorkspaceChanges: options.AllowUnverifiedWorkspaceChanges,
		CleanupPreparationErrors:        preparationErrors,
		Sessions:                        sessions,
		Worktrees:                       worktrees,
		WorktreeHeadOIDs:                worktreeHeadOIDs,
		WorktreeTaskDirNames:            worktreeTaskDirNames,
		WorktreeBranchMetadata:          worktreeBranchMetadata,
		StopTargets:                     persistStopTargets(stopTargets),
		TaskEnvironment:                 taskEnv,
		DeleteEnvironmentRow:            deleteEnvironmentRow,
		DiscardWorktreeChanges:          options.DiscardWorktreeChanges,
		WorkspaceCleanup:                options.WorkspaceCleanup,
		RetentionInventoryCaptured:      options.retainsWorkspace(),
		LegacyWorktreeCleanup:           s.hasLegacyWorktreeCleanup(),
		SSHTaskDirs:                     sshTaskDirs,
		WorkspaceID:                     task.WorkspaceID,
	}
	if err := s.captureUnavailableCleanupInventory(ctx, taskID, &snapshot); err != nil {
		return err
	}
	if err := s.captureRetainedRuntimeInventory(ctx, taskID, &snapshot); err != nil {
		return err
	}
	if taskResourceCleanupDeletesTask(job.Trigger) {
		snapshot.AgentHistory = append(snapshot.AgentHistory, s.captureQuickChatAgentHistory(ctx, task, sessions)...)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode task resource cleanup snapshot: %w", err)
	}
	if err := s.resourceCleanups.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, string(encoded)); err != nil {
		return fmt.Errorf("persist task resource cleanup snapshot: %w", err)
	}
	return nil
}
