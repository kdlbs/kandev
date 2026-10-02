package coordinator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// SubscribeTaskStalled subscribes svc to task.stalled events: for a
// workspace with at least one coordinator, it upserts the coordinator_stalls
// row and, on a change, publishes coordinator.updated once per coordinator
// of that workspace, in ListCoordinators order (docs/specs/coordinator/
// system-design/needs-you.md#stall-records).
func SubscribeTaskStalled(eventBus bus.EventBus, svc *Service, log *logger.Logger) (bus.Subscription, error) {
	h := &stallSubscriber{
		svc:      svc,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "coordinator-stall-subscriber")),
	}
	return eventBus.Subscribe(events.TaskStalled, h.handle)
}

type stallSubscriber struct {
	svc      *Service
	eventBus bus.EventBus
	logger   *logger.Logger
}

func (h *stallSubscriber) handle(ctx context.Context, event *bus.Event) error {
	stall, ok := stallFromEvent(event)
	if !ok {
		h.logger.Warn("dropping malformed task.stalled event")
		return nil
	}

	coordinators, err := h.svc.store.ListCoordinators(ctx, stall.WorkspaceID)
	if err != nil {
		return fmt.Errorf("list coordinators for stall: %w", err)
	}
	if len(coordinators) == 0 {
		return nil
	}

	matched, err := h.svc.store.UpsertStall(ctx, stall)
	if err != nil {
		return fmt.Errorf("upsert stall: %w", err)
	}
	if !matched {
		return nil
	}

	for _, c := range coordinators {
		count, err := h.svc.store.CountOpenProposals(ctx, c.ID, h.svc.phase2)
		if err != nil {
			h.logger.Warn("failed to count open proposals for coordinator.updated after stall",
				zap.String("workspace_id", stall.WorkspaceID),
				zap.String("coordinator_id", c.ID),
				zap.Error(err))
			continue
		}
		payload := NewCoordinatorUpdatedPayload(stall.WorkspaceID, c.ID, count)
		evt := bus.NewEvent(events.CoordinatorUpdated, "coordinator-stall-subscriber", payload)
		if err := h.eventBus.Publish(ctx, events.CoordinatorUpdated, evt); err != nil {
			h.logger.Warn("failed to publish coordinator.updated after stall",
				zap.String("workspace_id", stall.WorkspaceID),
				zap.String("coordinator_id", c.ID),
				zap.Error(err))
		}
	}
	return nil
}

// stallFromEvent parses a task.stalled payload (internal/task/service/
// active_session_stall.go): task_id, workspace_id, stalled_for (a
// time.Duration string) and last_event_at (RFC3339Nano). detection_only is
// ignored: this subscriber always records the stall regardless of its value.
// Any missing or unparsable field is a malformed event.
func stallFromEvent(event *bus.Event) (*Stall, bool) {
	if event == nil {
		return nil, false
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil, false
	}
	taskID, _ := data["task_id"].(string)
	workspaceID, _ := data["workspace_id"].(string)
	stalledFor, _ := data["stalled_for"].(string)
	lastEventAt, _ := data["last_event_at"].(string)
	taskID = strings.TrimSpace(taskID)
	workspaceID = strings.TrimSpace(workspaceID)
	if taskID == "" || workspaceID == "" || stalledFor == "" || lastEventAt == "" {
		return nil, false
	}
	duration, err := time.ParseDuration(stalledFor)
	if err != nil {
		return nil, false
	}
	lastEvent, err := time.Parse(time.RFC3339Nano, lastEventAt)
	if err != nil {
		return nil, false
	}
	return &Stall{
		TaskID:       taskID,
		WorkspaceID:  workspaceID,
		StalledForMs: duration.Milliseconds(),
		LastEventAt:  lastEvent,
		DetectedAt:   time.Now().UTC(),
	}, true
}
