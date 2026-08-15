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

func (c *Canceller) detachSessionBundles(ctx context.Context, sessionID string) int {
	// Draining live waiters and durable detachment are separate concerns. The
	// repository owns the atomic current-turn/status claim for persisted rows.
	c.store.CancelSession(sessionID)
	writeCtx := context.WithoutCancel(ctx)
	messages, err := c.repo.DetachActiveClarificationMessagesBySessionID(writeCtx, sessionID)
	if err != nil {
		c.logger.Warn("failed to detach current clarification bundles",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return 0
	}
	return c.publishChangedBundles(writeCtx, messages)
}

func (c *Canceller) expireSessionBundles(ctx context.Context, sessionID string) int {
	// The initial read discovers bundle identities only. Each transition below
	// rechecks that exact pending ID, current-turn ownership, and pending status
	// inside one UPDATE serialized with answers and successor turns.
	c.store.CancelSession(sessionID)
	writeCtx := context.WithoutCancel(ctx)
	messages, err := c.repo.FindActiveClarificationMessagesBySessionID(writeCtx, sessionID)
	if err != nil {
		c.logger.Warn("failed to load current clarification bundles for expiry",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return 0
	}
	pendingIDs := make(map[string]struct{})
	for _, message := range messages {
		if pendingID := stringFromMetadata(message.Metadata, "pending_id"); pendingID != "" {
			pendingIDs[pendingID] = struct{}{}
		}
	}
	changedBundles := 0
	for pendingID := range pendingIDs {
		changed, expireErr := c.repo.ExpireActiveClarificationBundle(writeCtx, sessionID, pendingID)
		if expireErr != nil {
			c.logger.Warn("failed to expire current clarification bundle",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.Error(expireErr))
			continue
		}
		if len(changed) == 0 {
			continue
		}
		changedBundles++
		for _, message := range changed {
			c.publishMessageUpdated(writeCtx, message)
		}
	}
	return changedBundles
}

func (c *Canceller) publishChangedBundles(ctx context.Context, messages []*taskmodels.Message) int {
	bundles := make(map[string]struct{})
	for _, message := range messages {
		if pendingID := stringFromMetadata(message.Metadata, "pending_id"); pendingID != "" {
			bundles[pendingID] = struct{}{}
		}
		c.publishMessageUpdated(ctx, message)
	}
	return len(bundles)
}

// DetachSessionAndNotify cancels in-memory WaitForResponse waiters for a session
// and marks DB clarification messages as pending with agent_disconnected=true.
// The overlay stays interactive; a late answer uses the acknowledged resume fallback path.
func (c *Canceller) DetachSessionAndNotify(ctx context.Context, sessionID string) int {
	return c.detachSessionBundles(ctx, sessionID)
}

// ExpireSessionAndNotify cancels in-memory waiters and marks clarification
// messages expired so the overlay closes and history shows a timed-out entry.
// TODO: wire this into terminal teardown paths that should close the overlay
// instead of preserving the deferred-answer UX.
func (c *Canceller) ExpireSessionAndNotify(ctx context.Context, sessionID string) int {
	return c.expireSessionBundles(ctx, sessionID)
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
		"updated_at":     msg.UpdatedAt.Format(time.RFC3339Nano),
		"metadata":       msg.Metadata,
	}

	event := bus.NewEvent(events.MessageUpdated, "clarification-canceller", data)
	if err := c.eventBus.Publish(ctx, events.MessageUpdated, event); err != nil {
		c.logger.Warn("failed to publish message.updated event",
			zap.String("message_id", msg.ID),
			zap.Error(err))
	}
}
