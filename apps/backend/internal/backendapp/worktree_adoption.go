package backendapp

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// registerWorktreeAdoptionSubscriber wires agentctl linked-worktree snapshots
// to the task service. Detection only ever attaches or retires roots; it never
// starts, resumes, or prompts an agent.
func registerWorktreeAdoptionSubscriber(eventBus bus.EventBus, taskSvc *taskservice.Service, log *logger.Logger) {
	if eventBus == nil || taskSvc == nil {
		return
	}
	_, err := eventBus.Subscribe(events.BuildWorkspaceWorktreesWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		payload := decodeWorktreeInventory(event)
		if payload == nil {
			return nil
		}
		taskSvc.HandleWorkspaceWorktreesObserved(ctx, payload)
		return nil
	})
	if err != nil {
		log.Warn("failed to subscribe to worktree inventory events", zap.Error(err))
	}
}

func decodeWorktreeInventory(event *bus.Event) *events.WorktreeInventoryData {
	if event == nil || event.Data == nil {
		return nil
	}
	if payload, ok := event.Data.(*events.WorktreeInventoryData); ok {
		return payload
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return nil
	}
	var payload events.WorktreeInventoryData
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil
	}
	return &payload
}
