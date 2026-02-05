package repository

import (
	"database/sql"

	"github.com/kandev/kandev/internal/analytics/repository/sqlite"
)

// Provide creates the SQLite analytics repository using the shared database connection.
func Provide(db *sql.DB) (Repository, func() error, error) {
	repo, err := sqlite.NewWithDB(db)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
