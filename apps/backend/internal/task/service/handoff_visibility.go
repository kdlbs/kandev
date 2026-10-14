package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

const taskVisibilitySchemaVersion = 1
const taskVisibilityDispositionDeleted = "deleted"
const taskVisibilityDispositionUnknown = "unknown"

// TaskVisibility is the compact, backend-authorized lifecycle projection.
// The durable receipt store may refine the disposition for deleted tasks;
// callers never receive task content or lifecycle internals.
type TaskVisibility struct {
	SchemaVersion  int                    `json:"schema_version"`
	TaskID         string                 `json:"task_id"`
	Identity       TaskVisibilityIdentity `json:"identity"`
	WorkspaceID    string                 `json:"workspace_id"`
	WorkflowID     string                 `json:"workflow_id"`
	WorkflowStepID string                 `json:"workflow_step_id"`
	State          string                 `json:"state"`
	Visibility     TaskVisibilityReceipt  `json:"visibility"`
}

type taskVisibilitySnapshotReader interface {
	GetTaskVisibilitySnapshot(context.Context, string, string) (*models.Task, *models.TaskLifecycleReceipt, error)
}

type TaskVisibilityIdentity struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
}
type TaskVisibilityReceipt struct {
	Disposition string    `json:"disposition"`
	ChangedAt   time.Time `json:"changed_at"`
	Revision    int64     `json:"revision"`
	Provenance  string    `json:"provenance"`
}

// GetTaskVisibilityForRequest resolves the target only after a compact-read scope.
func (s *HandoffService) GetTaskVisibilityForRequest(ctx context.Context, req RelatedReadRequest) (*TaskVisibility, error) {
	if req.Scope != RelatedReadScopeWorkspaceTaskTree && req.Scope != RelatedReadScopeTaskVisibility {
		return nil, relatedReadDenied(RelatedReadDenialScopeRequired, "workspace task visibility requires coordinator scope")
	}
	caller, err := s.tasks.GetTask(ctx, req.CallerTaskID)
	if err != nil || caller == nil {
		if errors.Is(err, repository.ErrTaskNotFound) || caller == nil {
			return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "caller task missing")
		}
		return nil, err
	}
	if req.CallerWorkspaceID != "" && caller.WorkspaceID != req.CallerWorkspaceID {
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "caller workspace changed")
	}
	snapshots, ok := s.tasks.(taskVisibilitySnapshotReader)
	if !ok {
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target lifecycle snapshot unavailable")
	}
	task, receipt, err := snapshots.GetTaskVisibilitySnapshot(ctx, caller.WorkspaceID, req.TargetTaskID)
	if err != nil {
		return nil, err
	}
	if task != nil && task.WorkspaceID != caller.WorkspaceID {
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target unavailable after authorization")
	}
	if receipt != nil {
		return taskVisibilityFromEvidence(task, receipt)
	}
	// A task without durable lifecycle evidence is deliberately indistinguishable
	// from an unavailable target. Reads must never synthesize a receipt.
	return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target lifecycle evidence unavailable")
}

func taskVisibilityFromEvidence(task *models.Task, receipt *models.TaskLifecycleReceipt) (*TaskVisibility, error) {
	if task == nil {
		if receipt.Disposition == taskVisibilityDispositionDeleted {
			return taskVisibilityFromReceipt(receipt), nil
		}
		return taskVisibilityUnknownFromEvidence(nil, receipt), nil
	}
	if receipt.Disposition == taskVisibilityDispositionDeleted {
		return taskVisibilityUnknownFromEvidence(task, receipt), nil
	}
	return taskVisibilityFromLiveTask(task, receipt), nil
}

func taskVisibilityUnknownFromEvidence(task *models.Task, receipt *models.TaskLifecycleReceipt) *TaskVisibility {
	var visibility *TaskVisibility
	if task == nil {
		visibility = taskVisibilityFromReceipt(receipt)
	} else {
		visibility = taskVisibilityFromLiveTask(task, receipt)
	}
	visibility.Visibility.Disposition = taskVisibilityDispositionUnknown
	return visibility
}

func taskVisibilityFromLiveTask(task *models.Task, receipt *models.TaskLifecycleReceipt) *TaskVisibility {
	return &TaskVisibility{SchemaVersion: taskVisibilitySchemaVersion, TaskID: task.ID,
		Identity:    TaskVisibilityIdentity{Identifier: task.Identifier, Title: task.Title},
		WorkspaceID: task.WorkspaceID, WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
		State: string(task.State), Visibility: TaskVisibilityReceipt{Disposition: receipt.Disposition,
			ChangedAt: receipt.ChangedAt.UTC(), Revision: receipt.Revision, Provenance: receipt.Provenance}}
}

func taskVisibilityFromReceipt(receipt *models.TaskLifecycleReceipt) *TaskVisibility {
	return &TaskVisibility{SchemaVersion: taskVisibilitySchemaVersion, TaskID: receipt.TaskID,
		Identity:    TaskVisibilityIdentity{Identifier: receipt.Identifier, Title: receipt.Title},
		WorkspaceID: receipt.WorkspaceID, WorkflowID: receipt.WorkflowID, WorkflowStepID: receipt.WorkflowStepID,
		State: receipt.State, Visibility: TaskVisibilityReceipt{Disposition: receipt.Disposition,
			ChangedAt: receipt.ChangedAt.UTC(), Revision: receipt.Revision, Provenance: receipt.Provenance}}
}
