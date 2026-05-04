package github

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// rateLimitSeedTimeout caps the startup `gh api rate_limit` probe so a slow
// CLI cannot delay backend boot.
const rateLimitSeedTimeout = 5 * time.Second

// attachRateTracker wires the rate tracker into whichever client
// implementation we ended up with, then seeds initial snapshots. The seed is
// best-effort — a missing CLI or network blip should not block startup.
func attachRateTracker(client Client, tracker *RateTracker, log *logger.Logger) {
	switch c := client.(type) {
	case *PATClient:
		c.WithRateTracker(tracker)
	case *GHClient:
		c.WithRateTracker(tracker)
		seedCtx, cancel := context.WithTimeout(context.Background(), rateLimitSeedTimeout)
		defer cancel()
		if err := c.FetchRateLimit(seedCtx); err != nil {
			log.Debug("seed gh rate limit failed", zap.Error(err))
		}
	}
}

// Provide creates the full GitHub integration stack: store, client, and service.
// Returns the service, a cleanup function, and any error.
func Provide(
	writer, reader *sqlx.DB,
	secrets SecretProvider,
	eventBus bus.EventBus,
	log *logger.Logger,
) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	client, authMethod, err := NewClient(ctx, secrets, log)
	if err != nil {
		// Not fatal — service works with nil client (unauthenticated).
		log.Warn("GitHub client not available: " + err.Error())
	}

	svc := NewService(client, authMethod, secrets, store, eventBus, log)
	attachRateTracker(client, svc.RateTracker(), log)
	svc.subscribeTaskEvents()

	cleanup := func() error {
		svc.unsubscribeTaskEvents()
		return nil
	}
	return svc, cleanup, nil
}
