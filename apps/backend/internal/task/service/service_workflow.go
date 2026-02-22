package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ApproveSessionResult contains the result of approving a session
type ApproveSessionResult struct {
	Session      *models.TaskSession
	Task         *models.Task
	WorkflowStep *wfmodels.WorkflowStep
}

// ApproveSession approves a session's current step and moves it to the next step.
// It reads the step's on_turn_complete actions to determine where to transition.
// If no transition actions are configured, it falls back to the next step by position.
func (s *Service) ApproveSession(ctx context.Context, sessionID string) (*ApproveSessionResult, error) {
	// Update review status to approved
	if err := s.sessions.UpdateSessionReviewStatus(ctx, sessionID, "approved"); err != nil {
		return nil, fmt.Errorf("failed to update review status: %w", err)
	}

	result := &ApproveSessionResult{}

	// Reload session to get updated review status
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session: %w", err)
	}
	result.Session = session

	// Get the current workflow step to check for transition targets
	if session.WorkflowStepID != nil && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, *session.WorkflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for approval transition",
				zap.String("workflow_step_id", *session.WorkflowStepID),
				zap.Error(err))
		} else {
			s.applyApprovalStepTransition(ctx, sessionID, step, result)
		}
	}

	return result, nil
}

// applyApprovalStepTransition resolves the next workflow step and updates session/task accordingly.
func (s *Service) applyApprovalStepTransition(ctx context.Context, sessionID string, step *wfmodels.WorkflowStep, result *ApproveSessionResult) {
	newStepID := s.resolveApprovalNextStep(ctx, step)

	if newStepID == "" {
		s.logger.Info("session approved but no next step found (may be at final step)",
			zap.String("session_id", sessionID),
			zap.String("current_step", step.ID),
			zap.String("current_step_name", step.Name))
		return
	}

	if err := s.sessions.UpdateSessionWorkflowStep(ctx, sessionID, newStepID); err != nil {
		s.logger.Error("failed to move session to next step after approval",
			zap.String("session_id", sessionID),
			zap.String("step_id", newStepID),
			zap.Error(err))
		return
	}

	// Also move the task to the new step
	if task, err := s.tasks.GetTask(ctx, result.Session.TaskID); err != nil {
		s.logger.Error("failed to get task for approval transition",
			zap.String("task_id", result.Session.TaskID),
			zap.Error(err))
	} else {
		task.WorkflowStepID = newStepID
		task.UpdatedAt = time.Now().UTC()
		if err := s.tasks.UpdateTask(ctx, task); err != nil {
			s.logger.Error("failed to move task to next step after approval",
				zap.String("task_id", result.Session.TaskID),
				zap.String("step_id", newStepID),
				zap.Error(err))
		} else {
			s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
			result.Task = task
		}
	}

	// Reload session with new step
	result.Session, _ = s.sessions.GetTaskSession(ctx, sessionID)

	// Get the new workflow step for the response
	if newStep, err := s.workflowStepGetter.GetStep(ctx, newStepID); err == nil {
		result.WorkflowStep = newStep
	}

	s.logger.Info("session approved and moved to next step",
		zap.String("session_id", sessionID),
		zap.String("from_step", step.ID),
		zap.String("to_step", newStepID))
}

// resolveApprovalNextStep determines the target step ID from a step's on_turn_complete actions,
// falling back to the next step by position when no actions are configured.
func (s *Service) resolveApprovalNextStep(ctx context.Context, step *wfmodels.WorkflowStep) string {
	var newStepID string
	for _, action := range step.Events.OnTurnComplete {
		switch action.Type {
		case "move_to_next":
			nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
			if err != nil {
				s.logger.Warn("failed to get next step by position",
					zap.String("workflow_id", step.WorkflowID),
					zap.Int("current_position", step.Position),
					zap.Error(err))
			} else if nextStep != nil {
				newStepID = nextStep.ID
			}
		case "move_to_step":
			if stepID, ok := action.Config["step_id"].(string); ok && stepID != "" {
				newStepID = stepID
			}
		}
		if newStepID != "" {
			return newStepID
		}
	}

	// Fall back to next step by position if no transition actions found
	if len(step.Events.OnTurnComplete) == 0 {
		nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
		if err != nil {
			s.logger.Warn("failed to get next step by position for fallback",
				zap.String("workflow_id", step.WorkflowID),
				zap.Int("current_position", step.Position),
				zap.Error(err))
		} else if nextStep != nil {
			s.logger.Info("using next step by position for approval transition (fallback)",
				zap.String("current_step", step.Name),
				zap.String("next_step", nextStep.Name))
			newStepID = nextStep.ID
		}
	}

	return newStepID
}

// UpdateTaskState updates the state of a task, moves it to the matching column,
// and publishes a task.state_changed event
func (s *Service) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	oldState := task.State

	if err := s.tasks.UpdateTaskState(ctx, id, state); err != nil {
		s.logger.Error("failed to update task state", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Reload task to get updated state
	task, err = s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("task state updated",
		zap.String("task_id", id),
		zap.String("workflow_step_id", task.WorkflowStepID),
		zap.String("state", string(task.State)))

	s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	s.logger.Info("task state changed",
		zap.String("task_id", id),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(state)))

	return task, nil
}

// UpdateTaskMetadata updates only the metadata of a task (merges with existing)
func (s *Service) UpdateTaskMetadata(ctx context.Context, id string, metadata map[string]interface{}) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Merge metadata (existing keys are preserved, new keys are added/updated)
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		task.Metadata[k] = v
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task metadata", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.logger.Debug("task metadata updated", zap.String("task_id", id), zap.Any("metadata", metadata))
	return task, nil
}

// MoveTaskResult contains the result of a MoveTask operation.
type MoveTaskResult struct {
	Task         *models.Task
	WorkflowStep *wfmodels.WorkflowStep
}

// MoveTask moves a task to a different workflow step and position
func (s *Service) MoveTask(ctx context.Context, id string, workflowID string, workflowStepID string, position int) (*MoveTaskResult, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.checkMoveTaskApproval(ctx, id, task.WorkflowStepID, workflowStepID); err != nil {
		return nil, err
	}

	oldState := task.State
	task.WorkflowID = workflowID
	task.WorkflowStepID = workflowStepID
	task.Position = position
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.syncActiveSessionWorkflowStep(ctx, id, task.WorkflowStepID)

	// Publish state_changed event if state changed, otherwise just updated
	if oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	} else {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	s.logger.Info("task moved",
		zap.String("task_id", id),
		zap.String("workflow_id", workflowID),
		zap.String("workflow_step_id", workflowStepID),
		zap.Int("position", position))

	result := &MoveTaskResult{Task: task}

	// Fetch the workflow step info if getter is available
	if s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for MoveTask response",
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
			// Don't fail the operation, just log and continue
		} else {
			result.WorkflowStep = step
		}
	}

	return result, nil
}

// checkMoveTaskApproval returns an error when the task's primary session has a pending
// review and the caller is trying to move it to a different step.
func (s *Service) checkMoveTaskApproval(ctx context.Context, taskID, currentStepID, targetStepID string) error {
	if currentStepID == targetStepID {
		return nil
	}
	primarySession, err := s.sessions.GetPrimarySessionByTaskID(ctx, taskID)
	if err != nil || primarySession == nil {
		return nil
	}
	if primarySession.ReviewStatus != nil && *primarySession.ReviewStatus == "pending" {
		return fmt.Errorf("task is pending approval - use Approve button to proceed or send a message to request changes")
	}
	return nil
}

// syncActiveSessionWorkflowStep updates the active session's workflow_step_id to match the
// task's new step when they differ. Failures are logged but do not abort the move.
func (s *Service) syncActiveSessionWorkflowStep(ctx context.Context, taskID, workflowStepID string) {
	if workflowStepID == "" {
		return
	}
	activeSession, err := s.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || activeSession == nil {
		return
	}
	if activeSession.WorkflowStepID != nil && *activeSession.WorkflowStepID == workflowStepID {
		return
	}
	if err := s.sessions.UpdateSessionWorkflowStep(ctx, activeSession.ID, workflowStepID); err != nil {
		s.logger.Warn("failed to update session workflow step after task move",
			zap.String("task_id", taskID),
			zap.String("session_id", activeSession.ID),
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
		return
	}
	s.logger.Info("updated session workflow step to match moved task",
		zap.String("task_id", taskID),
		zap.String("session_id", activeSession.ID),
		zap.String("workflow_step_id", workflowStepID))
}

// CountTasksByWorkflow returns the number of tasks in a workflow
func (s *Service) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	return s.tasks.CountTasksByWorkflow(ctx, workflowID)
}

// CountTasksByWorkflowStep returns the number of tasks in a workflow step
func (s *Service) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	return s.tasks.CountTasksByWorkflowStep(ctx, stepID)
}

// BulkMoveTasksResult contains the result of a BulkMoveTasks operation.
type BulkMoveTasksResult struct {
	MovedCount int
}

// BulkMoveTasks moves all tasks from a source workflow/step to a target workflow/step.
// If sourceStepID is empty, all tasks in the source workflow are moved.
func (s *Service) BulkMoveTasks(ctx context.Context, sourceWorkflowID, sourceStepID, targetWorkflowID, targetStepID string) (*BulkMoveTasksResult, error) {
	// Get the tasks to move
	var tasks []*models.Task
	var err error
	if sourceStepID != "" {
		tasks, err = s.tasks.ListTasksByWorkflowStep(ctx, sourceStepID)
	} else {
		tasks, err = s.tasks.ListTasks(ctx, sourceWorkflowID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks for bulk move: %w", err)
	}

	if len(tasks) == 0 {
		return &BulkMoveTasksResult{MovedCount: 0}, nil
	}

	now := time.Now().UTC()
	for i, task := range tasks {
		task.WorkflowID = targetWorkflowID
		task.WorkflowStepID = targetStepID
		task.Position = i
		task.UpdatedAt = now

		if err := s.tasks.UpdateTask(ctx, task); err != nil {
			s.logger.Error("failed to move task in bulk move",
				zap.String("task_id", task.ID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to move task %s: %w", task.ID, err)
		}

		// Update active session's workflow_step_id
		activeSession, err := s.sessions.GetActiveTaskSessionByTaskID(ctx, task.ID)
		if err == nil && activeSession != nil {
			if activeSession.WorkflowStepID == nil || *activeSession.WorkflowStepID != targetStepID {
				if err := s.sessions.UpdateSessionWorkflowStep(ctx, activeSession.ID, targetStepID); err != nil {
					s.logger.Warn("failed to update session workflow step during bulk move",
						zap.String("task_id", task.ID),
						zap.String("session_id", activeSession.ID),
						zap.Error(err))
				}
			}
		}

		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	s.logger.Info("bulk moved tasks",
		zap.String("source_workflow_id", sourceWorkflowID),
		zap.String("source_step_id", sourceStepID),
		zap.String("target_workflow_id", targetWorkflowID),
		zap.String("target_step_id", targetStepID),
		zap.Int("moved_count", len(tasks)))

	return &BulkMoveTasksResult{MovedCount: len(tasks)}, nil
}
