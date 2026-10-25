package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribeWorkflowStepCacheEvents keeps workflowStore's compiled-step cache
// fresh by invalidating on the same workflow_step.* events the queue
// reconciler already subscribes to (see subscribeWorkflowQueueEvents). This
// is the fast invalidation path; stepSpecCacheTTL is the correctness floor
// for writer paths (git-backed sync, step reorder, step-delete reference
// clearing) that don't publish these events.
func (s *Service) subscribeWorkflowStepCacheEvents() {
	if s.eventBus == nil {
		return
	}
	for _, eventType := range []string{events.WorkflowStepCreated, events.WorkflowStepUpdated, events.WorkflowStepDeleted} {
		if _, err := s.eventBus.Subscribe(eventType, s.handleWorkflowStepCacheInvalidation); err != nil {
			s.logger.Error("failed to subscribe to workflow step event for cache invalidation",
				zap.String("event_type", eventType), zap.Error(err))
		}
	}
}

func (s *Service) handleWorkflowStepCacheInvalidation(_ context.Context, event *bus.Event) error {
	if s.workflowStore == nil || s.workflowStore.stepCache == nil || event == nil {
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
	workflowID, _ := step["workflow_id"].(string)
	if stepID == "" && workflowID == "" {
		return nil
	}
	s.workflowStore.stepCache.invalidate(workflowID, stepID)
	return nil
}
