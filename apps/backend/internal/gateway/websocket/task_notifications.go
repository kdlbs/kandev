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
	b.subscribe(eventBus, events.WorkflowCreated, ws.ActionWorkflowCreated)
	b.subscribe(eventBus, events.WorkflowUpdated, ws.ActionWorkflowUpdated)
	b.subscribe(eventBus, events.WorkflowDeleted, ws.ActionWorkflowDeleted)
	b.subscribe(eventBus, events.TaskCreated, ws.ActionTaskCreated)
	b.subscribe(eventBus, events.TaskUpdated, ws.ActionTaskUpdated)
	b.subscribe(eventBus, events.TaskDeleted, ws.ActionTaskDeleted)
	b.subscribe(eventBus, events.TaskStateChanged, ws.ActionTaskStateChanged)
	b.subscribe(eventBus, events.TaskPlanCreated, ws.ActionTaskPlanCreated)
	b.subscribe(eventBus, events.TaskPlanUpdated, ws.ActionTaskPlanUpdated)
	b.subscribe(eventBus, events.TaskPlanDeleted, ws.ActionTaskPlanDeleted)
	b.subscribe(eventBus, events.RepositoryCreated, ws.ActionRepositoryCreated)
	b.subscribe(eventBus, events.RepositoryUpdated, ws.ActionRepositoryUpdated)
	b.subscribe(eventBus, events.RepositoryDeleted, ws.ActionRepositoryDeleted)
	b.subscribe(eventBus, events.RepositoryScriptCreated, ws.ActionRepositoryScriptCreated)
	b.subscribe(eventBus, events.RepositoryScriptUpdated, ws.ActionRepositoryScriptUpdated)
	b.subscribe(eventBus, events.RepositoryScriptDeleted, ws.ActionRepositoryScriptDeleted)
	b.subscribe(eventBus, events.ExecutorCreated, ws.ActionExecutorCreated)
	b.subscribe(eventBus, events.ExecutorUpdated, ws.ActionExecutorUpdated)
	b.subscribe(eventBus, events.ExecutorDeleted, ws.ActionExecutorDeleted)
	b.subscribe(eventBus, events.ExecutorProfileCreated, ws.ActionExecutorProfileCreated)
	b.subscribe(eventBus, events.ExecutorProfileUpdated, ws.ActionExecutorProfileUpdated)
	b.subscribe(eventBus, events.ExecutorProfileDeleted, ws.ActionExecutorProfileDeleted)
	b.subscribe(eventBus, events.ExecutorPrepareProgress, ws.ActionExecutorPrepareProgress)
	b.subscribe(eventBus, events.ExecutorPrepareCompleted, ws.ActionExecutorPrepareCompleted)
	b.subscribe(eventBus, events.EnvironmentCreated, ws.ActionEnvironmentCreated)
	b.subscribe(eventBus, events.EnvironmentUpdated, ws.ActionEnvironmentUpdated)
	b.subscribe(eventBus, events.EnvironmentDeleted, ws.ActionEnvironmentDeleted)
	b.subscribe(eventBus, events.TaskSessionStateChanged, ws.ActionSessionStateChanged)
	b.subscribe(eventBus, events.MessageAdded, ws.ActionSessionMessageAdded)
	b.subscribe(eventBus, events.MessageUpdated, ws.ActionSessionMessageUpdated)
	b.subscribe(eventBus, events.AgentctlStarting, ws.ActionSessionAgentctlStarting)
	b.subscribe(eventBus, events.AgentctlReady, ws.ActionSessionAgentctlReady)
	b.subscribe(eventBus, events.AgentctlError, ws.ActionSessionAgentctlError)
	b.subscribe(eventBus, events.TurnStarted, ws.ActionSessionTurnStarted)
	b.subscribe(eventBus, events.TurnCompleted, ws.ActionSessionTurnCompleted)
	b.subscribe(eventBus, events.MessageQueueStatusChanged, ws.ActionMessageQueueStatusChanged)

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
		// Try to extract session_id from event data (works for both map and struct types)
		var sessionID string
		if data, ok := event.Data.(map[string]interface{}); ok {
			sessionID, _ = data["session_id"].(string)
		} else if data, ok := event.Data.(interface{ GetSessionID() string }); ok {
			sessionID = data.GetSessionID()
		}

		// Debug logging for session state changes with metadata
		if action == ws.ActionSessionStateChanged {
			if data, ok := event.Data.(map[string]interface{}); ok {
				if metadata, ok := data["metadata"]; ok {
					b.logger.Info("received session.state_changed with metadata",
						zap.String("action", action),
						zap.String("session_id", sessionID),
						zap.Any("metadata", metadata))
				}
			}
		}

		switch action {
		case ws.ActionSessionAgentctlStarting, ws.ActionSessionAgentctlReady, ws.ActionSessionAgentctlError, ws.ActionSessionStateChanged:
			if sessionID != "" {
				b.hub.BroadcastToSession(sessionID, msg)
				return nil
			}
		case ws.ActionSessionMessageAdded, ws.ActionSessionMessageUpdated:
			if sessionID != "" {
				b.hub.BroadcastToSession(sessionID, msg)
				return nil
			}
		case ws.ActionMessageQueueStatusChanged:
			if sessionID != "" {
				b.hub.BroadcastToSession(sessionID, msg)
				return nil
			}
		case ws.ActionExecutorPrepareProgress, ws.ActionExecutorPrepareCompleted:
			if sessionID != "" {
				b.hub.BroadcastToSession(sessionID, msg)
				return nil
			}
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
