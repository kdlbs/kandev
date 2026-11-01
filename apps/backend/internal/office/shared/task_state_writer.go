package shared

import (
	"context"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskStateWriter applies state transitions through the task repository so
// completion gates and workflow-step checks stay atomic with each write.
type TaskStateWriter interface {
	UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error
	UpdateTaskStateIfWorkflowStep(ctx context.Context, taskID, expectedStepID string, state v1.TaskState) (bool, error)
	// UpdateTaskStateIfWorkflowStepAndState applies a transition only while
	// both caller-observed task fields still match.
	UpdateTaskStateIfWorkflowStepAndState(
		ctx context.Context, taskID, expectedStepID string, expectedState, state v1.TaskState,
	) (bool, error)
}
