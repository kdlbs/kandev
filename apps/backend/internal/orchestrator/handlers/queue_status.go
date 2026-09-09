package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"go.uber.org/zap"
)

// publishStatus emits the latest QueueStatus on the event bus so the frontend
// updates its store after every mutation.
func (h *QueueHandlers) publishStatus(ctx context.Context, sessionID string, admitted ...*messagequeue.QueuedMessage) {
	if h.eventBus == nil {
		return
	}
	status := h.queueService.GetStatus(ctx, sessionID)
	eventData := map[string]interface{}{
		fieldSessionID:  sessionID,
		"entries":       status.Entries,
		"count":         status.Count,
		fieldMax:        status.Max,
		"auto_run":      status.AutoRun,
		"merge_enabled": status.MergeEnabled,
	}
	if len(admitted) > 0 && admitted[0] != nil && admitted[0].QueuedBy != "" && !messagequeue.IsReservedQueuedBy(admitted[0].QueuedBy) {
		eventData["queued_by"] = admitted[0].QueuedBy
		eventData["queued_at"] = admitted[0].QueuedAt
	}
	if h.sessionTaskResolver != nil {
		if taskID, err := h.sessionTaskResolver(ctx, sessionID); err != nil {
			h.logger.Warn("resolve session task for queue status event", zap.String("session_id", sessionID), zap.Error(err))
		} else if taskID != "" {
			eventData["task_id"] = taskID
		}
	}
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(events.MessageQueueStatusChanged, "queue-handlers", eventData))
}
