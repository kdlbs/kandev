package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/runs/models"
)

// TaskStepResolver returns a task's current workflow step ID.
type TaskStepResolver func(ctx context.Context, taskID string) (string, error)

// SetTaskStepResolver wires the optional resolver behind
// models.QueuedWorkflowStepPayloadKey.
func (s *Service) SetTaskStepResolver(resolver TaskStepResolver) {
	s.taskStep = resolver
}

// stampTaskStep records the task's current step under
// models.QueuedWorkflowStepPayloadKey when the payload names a task and no
// workflow_step_id. A failed read leaves the step
// empty and never blocks the enqueue.
func (s *Service) stampTaskStep(ctx context.Context, payload map[string]any) {
	if s.taskStep == nil {
		return
	}
	if step, _ := payload["workflow_step_id"].(string); step != "" {
		return
	}
	taskID, _ := payload["task_id"].(string)
	if taskID == "" {
		return
	}
	step, err := s.taskStep(ctx, taskID)
	if err != nil {
		s.log.Debug("resolve task workflow step for run failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if step != "" {
		payload[models.QueuedWorkflowStepPayloadKey] = step
	}
}
