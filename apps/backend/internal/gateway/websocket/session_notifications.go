package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type SessionStreamBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

func RegisterSessionStreamNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *SessionStreamBroadcaster {
	b := &SessionStreamBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-session-stream-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.BuildGitWSEventWildcardSubject(), ws.ActionSessionGitEvent)
	b.subscribe(eventBus, events.BuildFileChangeWildcardSubject(), ws.ActionWorkspaceFileChanges)
	b.subscribe(eventBus, events.BuildShellOutputWildcardSubject(), ws.ActionSessionShellOutput)
	b.subscribe(eventBus, events.BuildShellExitWildcardSubject(), ws.ActionSessionShellOutput)
	b.subscribe(eventBus, events.BuildProcessOutputWildcardSubject(), ws.ActionSessionProcessOutput)
	b.subscribe(eventBus, events.BuildProcessStatusWildcardSubject(), ws.ActionSessionProcessStatus)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

func (b *SessionStreamBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *SessionStreamBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		sessionID := extractSessionID(event.Data)
		if sessionID == "" {
			return nil
		}
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build websocket notification", zap.String("action", action), zap.Error(err))
			return nil
		}
		b.hub.BroadcastToSession(sessionID, msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}

func extractSessionID(data interface{}) string {
	if data == nil {
		return ""
	}
	if typed, ok := data.(interface{ GetSessionID() string }); ok {
		return typed.GetSessionID()
	}
	if m, ok := data.(map[string]interface{}); ok {
		if sessionID, ok := m["session_id"].(string); ok {
			return sessionID
		}
	}
	return ""
}
