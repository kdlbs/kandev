package repository

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/repository/sqlite"
)

// Provide creates the SQLite analytics repository using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB) (Repository, func() error, error) {
	repo, err := sqlite.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
