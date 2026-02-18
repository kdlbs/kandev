package store

import "github.com/jmoiron/sqlx"

// Provide creates the SQLite prompts store using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB) (*sqliteRepository, func() error, error) {
	repo, err := newSQLiteRepositoryWithDB(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
