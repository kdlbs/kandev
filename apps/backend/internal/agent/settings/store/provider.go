package store

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide creates the SQLite agent settings store using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, log *logger.Logger) (*sqliteRepository, func() error, error) {
	repo, err := newSQLiteRepositoryWithDB(writer, reader, log)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
