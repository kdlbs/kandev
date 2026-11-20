package dashboard

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// persistTaskStatus writes the normalized status. A non-empty
// ExpectedCurrentState selects the automatic-completion compare-and-set; the
// automatic callers always supply the state they observed on the same read as
// ExpectedWorkflowStepID. Manual and approval-redirect paths keep their
// existing state-only or step-only guards.
func (s *DashboardService) persistTaskStatus(
	ctx context.Context, req TaskStatusUpdateRequest, state, expectedStepID string,
) (models.TaskStateChange, error) {
	if req.ExpectedCurrentState != "" {
		return s.persistAutomaticTaskStatus(ctx, req, state)
	}
	write := models.TaskStateWrite{State: v1.TaskState(state)}
	if state == stateCompleted {
		write.ExpectedWorkflowStepID = &expectedStepID
	}
	change, err := s.taskStateWriter.CommitTaskState(ctx, req.TaskID, write)
	if err != nil {
		return models.TaskStateChange{}, fmt.Errorf("update task state: %w", err)
	}
	if !change.Matched {
		if state == stateCompleted {
			return models.TaskStateChange{}, &WorkflowStepChangedError{TaskID: req.TaskID}
		}
		return models.TaskStateChange{}, fmt.Errorf("task state write was not admitted")
	}
	return change, nil
}

// persistAutomaticTaskStatus applies the automatic-completion
// compare-and-set on the caller-observed state and workflow step. The gate's
// later step read is deliberately not used here: if the two differ, the
// caller's step no longer matches and the write fails closed.
func (s *DashboardService) persistAutomaticTaskStatus(
	ctx context.Context, req TaskStatusUpdateRequest, state string,
) (models.TaskStateChange, error) {
	write := models.TaskStateWrite{
		State:                  v1.TaskState(state),
		ExpectedWorkflowStepID: &req.ExpectedWorkflowStepID,
	}
	expectedState := v1.TaskState(req.ExpectedCurrentState)
	write.ExpectedState = &expectedState
	change, err := s.taskStateWriter.CommitTaskState(ctx, req.TaskID, write)
	if err != nil {
		return models.TaskStateChange{}, fmt.Errorf("update automatic task state: %w", err)
	}
	if !change.Matched {
		return models.TaskStateChange{}, ErrAutomaticCompletionStale
	}
	return change, nil
}
