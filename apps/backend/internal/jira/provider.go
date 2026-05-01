package jira

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// Provide builds the Jira service. eventBus may be nil — used in tests and
// during early boot before the bus is ready; the service falls back to a
// no-op publish path. Cleanup is a no-op today — the service holds only
// in-memory client caches — but the signature mirrors other integration
// providers so callers can register it uniformly.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	svc := NewService(store, secrets, DefaultClientFactory, log)
	if eventBus != nil {
		svc.SetEventBus(eventBus)
	}
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}
