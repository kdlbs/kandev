package linear

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide builds the Linear service. Cleanup is a no-op today — the service
// holds only in-memory client caches — but the signature mirrors other
// integration providers so callers can register it uniformly.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	svc := NewService(store, secrets, DefaultClientFactory, log)
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}
