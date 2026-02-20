package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// fileChangeDebounceWindow is the time to wait before flushing batched file changes
const fileChangeDebounceWindow = 100 * time.Millisecond

// maxFileChangeBatchSize is the maximum number of file changes to batch before forcing a flush
const maxFileChangeBatchSize = 50

type SessionStreamBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger

	// File change debouncing
	fileChangeMu     sync.Mutex
	fileChangeBatch  map[string][]any       // sessionID -> list of file change payloads
	fileChangeTimers map[string]*time.Timer // sessionID -> debounce timer
}

func RegisterSessionStreamNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *SessionStreamBroadcaster {
	b := &SessionStreamBroadcaster{
		hub:              hub,
		logger:           log.WithFields(zap.String("component", "ws-session-stream-broadcaster")),
		fileChangeBatch:  make(map[string][]any),
		fileChangeTimers: make(map[string]*time.Timer),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.BuildGitWSEventWildcardSubject(), ws.ActionSessionGitEvent)
	// Use debounced subscription for file changes to batch high-frequency events
	b.subscribeFileChanges(eventBus, events.BuildFileChangeWildcardSubject(), ws.ActionWorkspaceFileChanges)
	b.subscribe(eventBus, events.BuildShellOutputWildcardSubject(), ws.ActionSessionShellOutput)
	b.subscribe(eventBus, events.BuildShellExitWildcardSubject(), ws.ActionSessionShellOutput)
	b.subscribe(eventBus, events.BuildProcessOutputWildcardSubject(), ws.ActionSessionProcessOutput)
	b.subscribe(eventBus, events.BuildProcessStatusWildcardSubject(), ws.ActionSessionProcessStatus)
	b.subscribe(eventBus, events.BuildAvailableCommandsWildcardSubject(), ws.ActionSessionAvailableCommands)
	b.subscribe(eventBus, events.BuildSessionModeWildcardSubject(), ws.ActionSessionModeChanged)

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

	// Stop all debounce timers
	b.fileChangeMu.Lock()
	for _, timer := range b.fileChangeTimers {
		timer.Stop()
	}
	b.fileChangeTimers = nil
	b.fileChangeBatch = nil
	b.fileChangeMu.Unlock()
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

// subscribeFileChanges subscribes to file change events with debouncing.
// Multiple file changes within the debounce window are batched into a single message.
func (b *SessionStreamBroadcaster) subscribeFileChanges(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		sessionID := extractSessionID(event.Data)
		if sessionID == "" {
			return nil
		}

		b.fileChangeMu.Lock()
		defer b.fileChangeMu.Unlock()

		// Add to batch
		b.fileChangeBatch[sessionID] = append(b.fileChangeBatch[sessionID], event.Data)

		// Force flush if batch is too large
		if len(b.fileChangeBatch[sessionID]) >= maxFileChangeBatchSize {
			b.flushFileChangesLocked(sessionID, action)
			return nil
		}

		// Reset or create debounce timer
		if timer, exists := b.fileChangeTimers[sessionID]; exists {
			timer.Stop()
		}
		b.fileChangeTimers[sessionID] = time.AfterFunc(fileChangeDebounceWindow, func() {
			b.fileChangeMu.Lock()
			defer b.fileChangeMu.Unlock()
			b.flushFileChangesLocked(sessionID, action)
		})

		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to file change events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}

// flushFileChangesLocked sends batched file changes. Must be called with fileChangeMu held.
func (b *SessionStreamBroadcaster) flushFileChangesLocked(sessionID, action string) {
	batch := b.fileChangeBatch[sessionID]
	if len(batch) == 0 {
		return
	}

	// Clear the batch
	delete(b.fileChangeBatch, sessionID)
	if timer, exists := b.fileChangeTimers[sessionID]; exists {
		timer.Stop()
		delete(b.fileChangeTimers, sessionID)
	}

	// Create a single notification with all changes
	payload := map[string]any{
		"session_id": sessionID,
		"changes":    batch,
	}

	msg, err := ws.NewNotification(action, payload)
	if err != nil {
		b.logger.Error("failed to build batched file change notification", zap.String("action", action), zap.Error(err))
		return
	}

	b.hub.BroadcastToSession(sessionID, msg)
	b.logger.Debug("flushed batched file changes",
		zap.String("session_id", sessionID),
		zap.Int("count", len(batch)))
}

func extractSessionID(data any) string {
	if data == nil {
		return ""
	}
	if typed, ok := data.(interface{ GetSessionID() string }); ok {
		return typed.GetSessionID()
	}
	if m, ok := data.(map[string]any); ok {
		if sessionID, ok := m["session_id"].(string); ok {
			return sessionID
		}
	}
	return ""
}
