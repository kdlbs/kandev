package store

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Provide creates the SQLite notification store using separate writer and reader pools.
func Provide(ctx context.Context, writer, reader *sqlx.DB) (*sqliteRepository, func() error, error) {
	repo, err := newSQLiteRepositoryWithDB(ctx, writer, reader)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
