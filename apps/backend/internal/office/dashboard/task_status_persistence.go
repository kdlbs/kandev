package dashboard

import (
	"context"
	"fmt"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// persistTaskStatus writes the normalized status. A non-empty
// ExpectedCurrentState selects the automatic-completion compare-and-set; the
// automatic callers always supply the state they observed on the same read as
// ExpectedWorkflowStepID. Manual and approval-redirect paths keep their
// existing state-only or step-only guards.
func (s *DashboardService) persistTaskStatus(ctx context.Context, req TaskStatusUpdateRequest, state, expectedStepID string) error {
	if req.ExpectedCurrentState != "" {
		return s.persistAutomaticTaskStatus(ctx, req, state)
	}
	if state != stateCompleted {
		if err := s.taskStateWriter.UpdateTaskState(ctx, req.TaskID, v1.TaskState(state)); err != nil {
			return fmt.Errorf("update task state: %w", err)
		}
		return nil
	}
	updated, err := s.taskStateWriter.UpdateTaskStateIfWorkflowStep(
		ctx, req.TaskID, expectedStepID, v1.TaskState(state),
	)
	if err != nil {
		return fmt.Errorf("update task state: %w", err)
	}
	if !updated {
		return &WorkflowStepChangedError{TaskID: req.TaskID}
	}
	return nil
}

// persistAutomaticTaskStatus applies the automatic-completion
// compare-and-set on the caller-observed state and workflow step. The gate's
// later step read is deliberately not used here: if the two differ, the
// caller's step no longer matches and the write fails closed.
func (s *DashboardService) persistAutomaticTaskStatus(ctx context.Context, req TaskStatusUpdateRequest, state string) error {
	updated, err := s.taskStateWriter.UpdateTaskStateIfWorkflowStepAndState(
		ctx, req.TaskID, req.ExpectedWorkflowStepID,
		v1.TaskState(req.ExpectedCurrentState), v1.TaskState(state),
	)
	if err != nil {
		return fmt.Errorf("update automatic task state: %w", err)
	}
	if !updated {
		return ErrAutomaticCompletionStale
	}
	return nil
}
