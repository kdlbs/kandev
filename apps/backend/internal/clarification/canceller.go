// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Canceller wraps Store with message-update side effects.
// When the agent's turn completes, it cancels pending clarifications
// and marks the database messages with agent_disconnected metadata.
type Canceller struct {
	store    *Store
	repo     messageStore
	eventBus EventBus
	logger   *logger.Logger
}

// NewCanceller creates a Canceller.
func NewCanceller(store *Store, repo messageStore, eventBus EventBus, log *logger.Logger) *Canceller {
	return &Canceller{
		store:    store,
		repo:     repo,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "clarification-canceller")),
	}
}

// CancelSessionAndNotify cancels all pending clarifications for a session,
// unblocking WaitForResponse callers, and marks the database messages
// with agent_disconnected=true so the frontend shows a deferred notice.
// Returns the number of cancelled clarifications.
func (c *Canceller) CancelSessionAndNotify(ctx context.Context, sessionID string) int {
	pendingIDs := c.store.CancelSession(sessionID)
	if len(pendingIDs) == 0 {
		return 0
	}

	for _, id := range pendingIDs {
		msg, err := c.repo.FindMessageByPendingID(ctx, id)
		if err != nil || msg == nil {
			c.logger.Debug("message not found for cancelled clarification",
				zap.String("pending_id", id),
				zap.Error(err))
			continue
		}
		if msg.Metadata == nil {
			msg.Metadata = map[string]any{}
		}
		msg.Metadata["agent_disconnected"] = true
		if err := c.repo.UpdateMessage(ctx, msg); err != nil {
			c.logger.Warn("failed to update message with agent_disconnected",
				zap.String("pending_id", id),
				zap.String("message_id", msg.ID),
				zap.Error(err))
			continue
		}

		// Publish message.updated event so the frontend picks up the metadata change
		c.publishMessageUpdated(ctx, msg)
	}

	return len(pendingIDs)
}

// publishMessageUpdated publishes a message.updated event to the event bus.
func (c *Canceller) publishMessageUpdated(ctx context.Context, msg *taskmodels.Message) {
	if c.eventBus == nil {
		return
	}

	msgType := string(msg.Type)
	if msgType == "" {
		msgType = "message"
	}

	data := map[string]any{
		"message_id":     msg.ID,
		"session_id":     msg.TaskSessionID,
		"task_id":        msg.TaskID,
		"turn_id":        msg.TurnID,
		"author_type":    string(msg.AuthorType),
		"author_id":      msg.AuthorID,
		"content":        msg.Content,
		"type":           msgType,
		"requests_input": msg.RequestsInput,
		"created_at":     msg.CreatedAt.Format(time.RFC3339),
		"metadata":       msg.Metadata,
	}

	event := bus.NewEvent(events.MessageUpdated, "clarification-canceller", data)
	if err := c.eventBus.Publish(ctx, events.MessageUpdated, event); err != nil {
		c.logger.Warn("failed to publish message.updated event",
			zap.String("message_id", msg.ID),
			zap.Error(err))
	}
}
