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

// markMessagesDetached unblocks WaitForResponse and keeps the clarification
// interactive: status stays pending and agent_disconnected is set so the UI
// can route a late answer through the event fallback path.
func (c *Canceller) markMessagesDetached(ctx context.Context, msgs []*taskmodels.Message, pendingID string) {
	for _, msg := range msgs {
		if msg.Metadata == nil {
			msg.Metadata = map[string]any{}
		}
		if current, _ := msg.Metadata["status"].(string); isTerminalStatus(current) {
			continue
		}
		msg.Metadata["agent_disconnected"] = true
		if err := c.repo.UpdateMessage(ctx, msg); err != nil {
			c.logger.Warn("failed to update message with detached status",
				zap.String("pending_id", pendingID),
				zap.String("message_id", msg.ID),
				zap.Error(err))
			continue
		}
		c.publishMessageUpdated(ctx, msg)
	}
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

func (c *Canceller) detachSessionBundles(ctx context.Context, sessionID string) int {
	pendingIDs := c.store.CancelSession(sessionID)

	handled := make(map[string]bool, len(pendingIDs))
	for _, id := range pendingIDs {
		msgs, err := c.repo.FindMessagesByPendingID(ctx, id)
		if err != nil || len(msgs) == 0 {
			c.logger.Debug("messages not found for detached clarification",
				zap.String("pending_id", id),
				zap.Error(err))
			continue
		}
		c.markMessagesDetached(ctx, msgs, id)
		handled[id] = true
	}

	msgs, err := c.repo.FindPendingClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil || len(msgs) == 0 {
		return len(pendingIDs)
	}

	byPendingID := make(map[string][]*taskmodels.Message)
	for _, msg := range msgs {
		pid := stringFromMetadata(msg.Metadata, "pending_id")
		if pid == "" || handled[pid] {
			continue
		}
		byPendingID[pid] = append(byPendingID[pid], msg)
	}
	for pid, bundle := range byPendingID {
		c.markMessagesDetached(ctx, bundle, pid)
	}
	return len(pendingIDs) + len(byPendingID)
}

// DetachSessionAndNotify cancels in-memory WaitForResponse waiters for a session
// and marks DB clarification messages as pending with agent_disconnected=true.
// The overlay stays interactive; a late answer uses the event fallback path.
func (c *Canceller) DetachSessionAndNotify(ctx context.Context, sessionID string) int {
	return c.detachSessionBundles(ctx, sessionID)
}

// Deprecated: use DetachSessionAndNotify. This alias remains so older tests and
// integrations keep the pre-rename entrypoint while detach semantics settle.
func (c *Canceller) CancelSessionAndNotify(ctx context.Context, sessionID string) int {
	return c.DetachSessionAndNotify(ctx, sessionID)
}

// ExpireSessionAndNotify cancels in-memory waiters and marks clarification
// messages expired so the overlay closes and history shows a timed-out entry.
// TODO: wire this into terminal teardown paths that should close the overlay
// instead of preserving the deferred-answer UX.
func (c *Canceller) ExpireSessionAndNotify(ctx context.Context, sessionID string) int {
	pendingIDs := c.store.CancelSession(sessionID)

	handled := make(map[string]bool, len(pendingIDs))
	for _, id := range pendingIDs {
		msgs, err := c.repo.FindMessagesByPendingID(ctx, id)
		if err != nil || len(msgs) == 0 {
			continue
		}
		c.markMessagesExpired(ctx, msgs, id)
		handled[id] = true
	}

	msgs, err := c.repo.FindPendingClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil || len(msgs) == 0 {
		return len(pendingIDs)
	}

	byPendingID := make(map[string][]*taskmodels.Message)
	for _, msg := range msgs {
		pid := stringFromMetadata(msg.Metadata, "pending_id")
		if pid == "" || handled[pid] {
			continue
		}
		byPendingID[pid] = append(byPendingID[pid], msg)
	}
	for pid, bundle := range byPendingID {
		c.markMessagesExpired(ctx, bundle, pid)
	}
	return len(pendingIDs) + len(byPendingID)
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
