package service

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/models"
)

// RetainedDeletionResult describes a committed deletion independently of shutdown.
type RetainedDeletionResult struct {
	CleanupOperationIDs     []string                    `json:"cleanup_operation_ids,omitempty"`
	WorkspaceCleanupPending bool                        `json:"workspace_cleanup_pending"`
	Success                 bool                        `json:"success"`
	WorkspaceCleanup        models.WorkspaceCleanupMode `json:"workspace_cleanup"`
	RetentionOperationIDs   []string                    `json:"retention_operation_ids"`
	RuntimeStopPending      bool                        `json:"runtime_stop_pending"`
}

func (s *Service) RetainedTaskDeletionResult(ctx context.Context, taskIDs []string) RetainedDeletionResult {
	result := RetainedDeletionResult{Success: true, WorkspaceCleanup: models.WorkspaceCleanupRetain, RetentionOperationIDs: []string{}}
	inspector, ok := s.resourceCleanups.(taskResourceCleanupTaskInspector)
	if !ok {
		result.RuntimeStopPending = true
		return result
	}
	for _, id := range taskIDs {
		jobs, err := inspector.ListTaskResourceCleanupJobs(ctx, id)
		if err != nil {
			result.RuntimeStopPending = true
			continue
		}
		for _, job := range jobs {
			var snapshot taskResourceCleanupSnapshot
			if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
				result.RuntimeStopPending = true
				continue
			}
			if snapshot.WorkspaceCleanup != models.WorkspaceCleanupRetain {
				continue
			}
			result.RetentionOperationIDs = append(result.RetentionOperationIDs, job.OperationID)
			if len(snapshot.StopTargets) > 0 && job.State != models.TaskResourceCleanupStateSucceeded {
				result.RuntimeStopPending = true
			}
		}
	}
	return result
}

// CommittedTaskDeletionResult reports physical cleanup separately from task removal.
func (s *Service) CommittedTaskDeletionResult(ctx context.Context, taskIDs []string, options DeleteTaskOptions) RetainedDeletionResult {
	if options.retainsWorkspace() {
		return s.RetainedTaskDeletionResult(ctx, taskIDs)
	}
	result := RetainedDeletionResult{Success: true, WorkspaceCleanup: models.WorkspaceCleanupDelete, CleanupOperationIDs: []string{}}
	inspector, ok := s.resourceCleanups.(taskResourceCleanupTaskInspector)
	if !ok {
		result.WorkspaceCleanupPending = true
		result.RuntimeStopPending = true
		return result
	}
	for _, id := range taskIDs {
		jobs, err := inspector.ListTaskResourceCleanupJobs(ctx, id)
		if err != nil {
			result.WorkspaceCleanupPending = true
			result.RuntimeStopPending = true
			continue
		}
		for _, job := range jobs {
			if job.State == models.TaskResourceCleanupStateCancelled {
				continue
			}
			var snapshot taskResourceCleanupSnapshot
			if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
				result.WorkspaceCleanupPending = true
				result.RuntimeStopPending = true
				continue
			}
			if !snapshot.AllowUnverifiedWorkspaceChanges {
				continue
			}
			result.CleanupOperationIDs = append(result.CleanupOperationIDs, job.OperationID)
			if job.State != models.TaskResourceCleanupStateSucceeded {
				result.WorkspaceCleanupPending = true
				result.RuntimeStopPending = result.RuntimeStopPending || (len(snapshot.StopTargets) > 0 && !snapshot.RuntimeStopCompleted)
			}
		}
	}
	return result
}
