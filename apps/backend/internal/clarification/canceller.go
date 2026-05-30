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

// isTerminalStatus returns true for statuses that should not be overwritten.
func isTerminalStatus(status string) bool {
	switch status {
	case string(StatusAnswered), string(StatusExpired), string(StatusRejected), string(StatusCancelled):
		return true
	}
	return false
}

// markMessagesExpired updates the given messages to status=expired and publishes
// a message.updated event for each one. It is idempotent: already-terminal
// messages are skipped.
func (c *Canceller) markMessagesExpired(ctx context.Context, msgs []*taskmodels.Message, pendingID string) {
	for _, msg := range msgs {
		if msg.Metadata == nil {
			msg.Metadata = map[string]any{}
		}
		if current, _ := msg.Metadata["status"].(string); isTerminalStatus(current) {
			continue
		}
		msg.Metadata["agent_disconnected"] = true
		msg.Metadata["status"] = string(StatusExpired)
		if err := c.repo.UpdateMessage(ctx, msg); err != nil {
			c.logger.Warn("failed to update message with expired status",
				zap.String("pending_id", pendingID),
				zap.String("message_id", msg.ID),
				zap.Error(err))
			continue
		}
		c.publishMessageUpdated(ctx, msg)
	}
}

// CancelSessionAndNotify cancels all pending clarifications for a session,
// unblocking WaitForResponse callers, and marks the database messages
// as expired (with agent_disconnected=true for context) so the frontend
// closes the interactive overlay and renders a "Timed out" history entry.
// Returns the number of cancelled clarifications.
//
// Setting status=expired also prevents a UX bug where clicking the overlay's
// X button after the agent moved on would trigger a new turn via the
// respond handler's event fallback path (rejected responses were being
// forwarded to the orchestrator as "User declined to answer").
func (c *Canceller) CancelSessionAndNotify(ctx context.Context, sessionID string) int {
	pendingIDs := c.store.CancelSession(sessionID)
	if len(pendingIDs) > 0 {
		for _, id := range pendingIDs {
			msgs, err := c.repo.FindMessagesByPendingID(ctx, id)
			if err != nil || len(msgs) == 0 {
				c.logger.Debug("messages not found for cancelled clarification",
					zap.String("pending_id", id),
					zap.Error(err))
				continue
			}
			c.markMessagesExpired(ctx, msgs, id)
		}
		return len(pendingIDs)
	}

	// Defense-in-depth: the in-memory store was already drained by a racing
	// cancel path (e.g. the MCP-timeout cleanup). Find still-pending
	// clarification messages in the DB and mark them expired directly.
	msgs, err := c.repo.FindPendingClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil || len(msgs) == 0 {
		return 0
	}

	// Group by pending_id so we return a bundle count.
	byPendingID := make(map[string][]*taskmodels.Message)
	for _, msg := range msgs {
		pid := stringFromMetadata(msg.Metadata, "pending_id")
		if pid == "" {
			continue
		}
		byPendingID[pid] = append(byPendingID[pid], msg)
	}
	for pid, bundle := range byPendingID {
		c.markMessagesExpired(ctx, bundle, pid)
	}
	return len(byPendingID)
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
