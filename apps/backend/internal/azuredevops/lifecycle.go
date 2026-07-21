package azuredevops

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// LifecycleCleanup owns subscriptions that remove Azure DevOps data alongside
// the Kandev resources that own it.
type LifecycleCleanup struct {
	subscriptions []bus.Subscription
}

// RegisterLifecycleCleanup subscribes to task and workspace deletion events.
func RegisterLifecycleCleanup(eventBus bus.EventBus, service *Service) (*LifecycleCleanup, error) {
	if eventBus == nil {
		return nil, errors.New("azure devops lifecycle: event bus is required")
	}
	if service == nil || service.store == nil {
		return nil, errors.New("azure devops lifecycle: service is required")
	}
	cleanup := &LifecycleCleanup{}
	taskSub, err := eventBus.Subscribe(events.TaskDeleted, service.handleTaskDeleted)
	if err != nil {
		return nil, fmt.Errorf("subscribe to task deletion: %w", err)
	}
	cleanup.subscriptions = append(cleanup.subscriptions, taskSub)
	workspaceSub, err := eventBus.Subscribe(events.WorkspaceDeleted, service.handleWorkspaceDeleted)
	if err != nil {
		_ = cleanup.Close()
		return nil, fmt.Errorf("subscribe to workspace deletion: %w", err)
	}
	cleanup.subscriptions = append(cleanup.subscriptions, workspaceSub)
	return cleanup, nil
}

func (s *Service) handleTaskDeleted(ctx context.Context, event *bus.Event) error {
	taskID := lifecycleResourceID(event, "task_id")
	if taskID == "" {
		return nil
	}
	if err := s.store.DeleteTaskPRsByTask(ctx, taskID); err != nil {
		return fmt.Errorf("delete Azure DevOps task PRs for task %q: %w", taskID, err)
	}
	return nil
}

func (s *Service) handleWorkspaceDeleted(ctx context.Context, event *bus.Event) error {
	workspaceID := lifecycleResourceID(event, "id")
	if workspaceID == "" {
		return nil
	}
	if err := s.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return fmt.Errorf("delete Azure DevOps config for workspace %q: %w", workspaceID, err)
	}
	return nil
}

func lifecycleResourceID(event *bus.Event, key string) string {
	if event == nil {
		return ""
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := data[key].(string)
	return id
}

// Close releases every lifecycle subscription.
func (c *LifecycleCleanup) Close() error {
	if c == nil {
		return nil
	}
	var result error
	for _, subscription := range c.subscriptions {
		result = errors.Join(result, subscription.Unsubscribe())
	}
	c.subscriptions = nil
	return result
}
