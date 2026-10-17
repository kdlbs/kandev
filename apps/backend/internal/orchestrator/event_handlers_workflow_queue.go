package orchestrator

import (
	"context"
	"strconv"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
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

// queuePromotionEntryID returns the ledger identity of the step entry that a
// queue promotion delivers. A task queued on a full step entered that step when
// it was queued, so the same-step write that later admits it records no
// transition and its delivery carries no id. That write pins the queued entry
// on the promotion marker, and the pinned row is the entry identity for the
// delivery. A marker without a pinned row falls back to the task's latest
// ledger row.
func (s *Service) queuePromotionEntryID(ctx context.Context, taskID string, deliveredID int64, promotionToken interface{}) int64 {
	if deliveredID > 0 {
		return deliveredID
	}
	if pinnedID := queuePromotionStampedEntryID(promotionToken); pinnedID > 0 {
		return pinnedID
	}
	reader, ok := s.repo.(workflowStepTransitionReader)
	if !ok {
		return deliveredID
	}
	latestID, err := reader.GetLatestTaskStepTransitionID(ctx, taskID)
	if err != nil {
		s.logger.Warn("task.queue_promoted: failed to read the promoted step entry",
			zap.String("task_id", taskID), zap.Error(err))
		return deliveredID
	}
	return latestID
}

// queuePromotionStampedEntryID reads the entry row a same-step promotion
// pinned on its marker. Decimal strings preserve the full ledger range through
// metadata JSON; numeric markers remain readable for compatibility.
func queuePromotionStampedEntryID(promotionToken interface{}) int64 {
	marker, ok := promotionToken.(map[string]interface{})
	if !ok {
		return 0
	}
	switch value := marker[models.QueuePromotionEntryTransitionIDKey].(type) {
	case string:
		entryID, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0
		}
		return entryID
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}
