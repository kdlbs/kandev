package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

func RegisterTaskNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *TaskEventBroadcaster {
	b := &TaskEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-task-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.WorkspaceCreated, ws.ActionWorkspaceCreated)
	b.subscribe(eventBus, events.WorkspaceUpdated, ws.ActionWorkspaceUpdated)
	b.subscribe(eventBus, events.WorkspaceDeleted, ws.ActionWorkspaceDeleted)
	b.subscribe(eventBus, events.BoardCreated, ws.ActionBoardCreated)
	b.subscribe(eventBus, events.BoardUpdated, ws.ActionBoardUpdated)
	b.subscribe(eventBus, events.BoardDeleted, ws.ActionBoardDeleted)
	b.subscribe(eventBus, events.ColumnCreated, ws.ActionColumnCreated)
	b.subscribe(eventBus, events.ColumnUpdated, ws.ActionColumnUpdated)
	b.subscribe(eventBus, events.ColumnDeleted, ws.ActionColumnDeleted)
	b.subscribe(eventBus, events.TaskCreated, ws.ActionTaskCreated)
	b.subscribe(eventBus, events.TaskUpdated, ws.ActionTaskUpdated)
	b.subscribe(eventBus, events.TaskDeleted, ws.ActionTaskDeleted)
	b.subscribe(eventBus, events.TaskStateChanged, ws.ActionTaskStateChanged)
	b.subscribe(eventBus, events.RepositoryCreated, ws.ActionRepositoryCreated)
	b.subscribe(eventBus, events.RepositoryUpdated, ws.ActionRepositoryUpdated)
	b.subscribe(eventBus, events.RepositoryDeleted, ws.ActionRepositoryDeleted)
	b.subscribe(eventBus, events.RepositoryScriptCreated, ws.ActionRepositoryScriptCreated)
	b.subscribe(eventBus, events.RepositoryScriptUpdated, ws.ActionRepositoryScriptUpdated)
	b.subscribe(eventBus, events.RepositoryScriptDeleted, ws.ActionRepositoryScriptDeleted)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

func (b *TaskEventBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *TaskEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build websocket notification", zap.String("action", action), zap.Error(err))
			return nil
		}
		b.hub.Broadcast(msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
