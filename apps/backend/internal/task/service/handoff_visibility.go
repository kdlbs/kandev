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

type taskLifecycleReceiptReader interface {
	GetTaskLifecycleReceipt(context.Context, string, string) (*models.TaskLifecycleReceipt, error)
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

// GetTaskVisibilityForRequest resolves the target only after the same
// workspace-tree authorization used by compact related-task reads.
func (s *HandoffService) GetTaskVisibilityForRequest(ctx context.Context, req RelatedReadRequest) (*TaskVisibility, error) {
	if req.Scope != RelatedReadScopeWorkspaceTaskTree {
		return nil, relatedReadDenied(RelatedReadDenialScopeRequired, "workspace task visibility requires coordinator scope")
	}
	caller, err := s.tasks.GetTask(ctx, req.CallerTaskID)
	if err != nil || caller == nil {
		if errors.Is(err, repository.ErrTaskNotFound) || caller == nil {
			return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "caller task missing")
		}
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, req.TargetTaskID)
	if err != nil && !errors.Is(err, repository.ErrTaskNotFound) {
		return nil, err
	}
	if task != nil && task.WorkspaceID != caller.WorkspaceID {
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target unavailable after authorization")
	}
	if receipts, ok := s.tasks.(taskLifecycleReceiptReader); ok {
		receipt, receiptErr := receipts.GetTaskLifecycleReceipt(ctx, caller.WorkspaceID, req.TargetTaskID)
		if receiptErr != nil {
			return nil, receiptErr
		}
		if receipt != nil {
			return taskVisibilityFromEvidence(task, receipt)
		}
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
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target lifecycle evidence is inconsistent")
	}
	if receipt.Disposition == taskVisibilityDispositionDeleted {
		return nil, relatedReadDenied(RelatedReadDenialTargetUnavailable, "target lifecycle evidence is inconsistent")
	}
	return taskVisibilityFromLiveTask(task, receipt), nil
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
