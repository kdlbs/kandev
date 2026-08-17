// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type clarificationBundlePublisher interface {
	PublishClarificationBundleUpdates(context.Context, []*taskmodels.Message) error
}

// Canceller wraps Store with message-update side effects.
// When the agent's turn completes, it cancels pending clarifications
// and marks the database messages with agent_disconnected metadata.
type Canceller struct {
	store     *Store
	repo      cancellationMessageStore
	publisher clarificationBundlePublisher
	logger    *logger.Logger
}

// NewCanceller creates a Canceller.
func NewCanceller(
	store *Store,
	repo cancellationMessageStore,
	publisher clarificationBundlePublisher,
	log *logger.Logger,
) *Canceller {
	return &Canceller{
		store:     store,
		repo:      repo,
		publisher: publisher,
		logger:    log.WithFields(zap.String("component", "clarification-canceller")),
	}
}

func (c *Canceller) detachSessionBundles(ctx context.Context, sessionID string) (int, error) {
	// Draining live waiters and durable detachment are separate concerns. The
	// repository owns the atomic current-turn/status claim for persisted rows.
	c.store.CancelSession(sessionID)
	writeCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	messages, err := c.repo.DetachActiveClarificationMessagesBySessionID(writeCtx, sessionID)
	if err != nil {
		c.logger.Error("failed to detach current clarification bundles",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return 0, fmt.Errorf("detach current clarification bundles: %w", err)
	}
	return c.publishChangedBundles(writeCtx, messages), nil
}

func (c *Canceller) expireSessionBundles(ctx context.Context, sessionID string) (int, error) {
	// The initial read discovers bundle identities only. Each transition below
	// rechecks that exact pending ID, current-turn ownership, and pending status
	// inside one UPDATE serialized with answers and successor turns.
	c.store.CancelSession(sessionID)
	writeCtx, cancel := clarificationPersistenceContextPreservingDeadline(ctx)
	defer cancel()
	messages, err := c.repo.FindActiveClarificationMessagesBySessionID(writeCtx, sessionID)
	if err != nil {
		c.logger.Warn("failed to load current clarification bundles for expiry",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return 0, fmt.Errorf("load current clarification bundles for expiry: %w", err)
	}
	pendingIDs := make(map[string]struct{})
	for _, message := range messages {
		if pendingID := stringFromMetadata(message.Metadata, "pending_id"); pendingID != "" {
			pendingIDs[pendingID] = struct{}{}
		}
	}
	changedBundles := 0
	var expiryErr error
	// Bundle expiry is intentionally best effort, not all-or-nothing. A bundle
	// that commits stays terminal even if a sibling write fails; the joined
	// error tells the caller to retry, and that retry only sees bundles still
	// pending under the repository's guarded transition.
	for pendingID := range pendingIDs {
		changed, expireErr := c.repo.ExpireActiveClarificationBundle(writeCtx, sessionID, pendingID)
		if expireErr != nil {
			c.logger.Warn("failed to expire current clarification bundle",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.Error(expireErr))
			expiryErr = errors.Join(
				expiryErr,
				fmt.Errorf("expire clarification bundle %s: %w", pendingID, expireErr),
			)
			continue
		}
		if len(changed) == 0 {
			continue
		}
		changedBundles++
		c.publishBundleUpdates(writeCtx, changed)
	}
	return changedBundles, expiryErr
}

func (c *Canceller) publishChangedBundles(ctx context.Context, messages []*taskmodels.Message) int {
	bundles := make(map[string]struct{})
	for _, message := range messages {
		if pendingID := stringFromMetadata(message.Metadata, "pending_id"); pendingID != "" {
			bundles[pendingID] = struct{}{}
		}
	}
	c.publishBundleUpdates(ctx, messages)
	return len(bundles)
}

// DetachSessionAndNotify cancels in-memory WaitForResponse waiters for a session
// and marks DB clarification messages as pending with agent_disconnected=true.
// The overlay stays interactive; a late answer uses the acknowledged resume fallback path.
func (c *Canceller) DetachSessionAndNotify(ctx context.Context, sessionID string) (int, error) {
	return c.detachSessionBundles(ctx, sessionID)
}

// ExpireSessionAndNotify cancels in-memory waiters and marks clarification
// messages expired so the overlay closes and history shows a timed-out entry.
func (c *Canceller) ExpireSessionAndNotify(ctx context.Context, sessionID string) (int, error) {
	return c.expireSessionBundles(ctx, sessionID)
}

// publishBundleUpdates routes committed rows through the task service so every
// message event carries the authoritative pending-action projection.
func (c *Canceller) publishBundleUpdates(ctx context.Context, messages []*taskmodels.Message) {
	if c.publisher == nil || len(messages) == 0 {
		return
	}
	if err := c.publisher.PublishClarificationBundleUpdates(ctx, messages); err != nil {
		c.logger.Warn("failed to publish clarification bundle updates",
			zap.Int("message_count", len(messages)),
			zap.Error(err))
	}
}
