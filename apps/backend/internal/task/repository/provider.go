package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Provide creates the SQLite repository using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, log *logger.Logger) (*sqlite.Repository, func() error, error) {
	return ProvideContext(context.Background(), writer, reader, log)
}

// ProvideContext creates the SQLite repository while observing startup
// cancellation between schema steps and during context-aware migrations.
func ProvideContext(ctx context.Context, writer, reader *sqlx.DB, log *logger.Logger) (*sqlite.Repository, func() error, error) {
	repo, err := sqlite.NewWithDBContext(ctx, writer, reader, log)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
