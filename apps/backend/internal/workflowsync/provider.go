package workflowsync

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide builds the workflow sync service. Cleanup is a no-op today but the
// signature mirrors other integration providers so callers can register it
// uniformly.
func Provide(writer, reader *sqlx.DB, clients ClientProvider, applier Applier, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	svc := NewService(store, clients, applier, log)
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}
