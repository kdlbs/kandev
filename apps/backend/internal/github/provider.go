package github

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

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

	svc := NewService(client, authMethod, store, eventBus, log)

	cleanup := func() error { return nil }
	return svc, cleanup, nil
}
