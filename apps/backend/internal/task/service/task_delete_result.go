package service

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
)

// RetainedDeletionResult describes a committed deletion independently of shutdown.
type RetainedDeletionResult struct {
	Success               bool                        `json:"success"`
	WorkspaceCleanup      models.WorkspaceCleanupMode `json:"workspace_cleanup"`
	RetentionOperationIDs []string                    `json:"retention_operation_ids"`
	RuntimeStopPending    bool                        `json:"runtime_stop_pending"`
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
