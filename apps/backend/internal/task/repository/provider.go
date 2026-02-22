package repository

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Provide creates the SQLite repository using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB) (*sqlite.Repository, func() error, error) {
	repo, err := sqlite.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
