package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribeWorkflowQueueEvents keeps queue admission in sync with workflow
// configuration changes. This also covers changing a step from a bounded WIP
// limit to unlimited, or changing its feeder.
func (s *Service) subscribeWorkflowQueueEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.WorkflowStepUpdated, s.handleWorkflowStepQueueUpdate); err != nil {
		s.logger.Error("failed to subscribe to workflow step updates for queue reconciliation", zap.Error(err))
	}
}

func (s *Service) handleWorkflowStepQueueUpdate(ctx context.Context, event *bus.Event) error {
	if s.workflowStore == nil || event == nil {
		return nil
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}
	step, ok := data["step"].(map[string]interface{})
	if !ok {
		return nil
	}
	stepID, _ := step["id"].(string)
	if stepID != "" {
		s.workflowStore.pullNextTaskOnVacate(ctx, stepID, "")
	}
	return nil
}
