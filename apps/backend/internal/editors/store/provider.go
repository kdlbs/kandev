package store

import "github.com/jmoiron/sqlx"

// Provide creates the SQLite editors store using the shared database connection.
func Provide(db *sqlx.DB) (*sqliteRepository, func() error, error) {
	repo, err := newSQLiteRepositoryWithDB(db)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
