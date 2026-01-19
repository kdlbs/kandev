package store

import "database/sql"

// Provide creates the SQLite agent settings store using the shared database connection.
func Provide(db *sql.DB) (*sqliteRepository, func() error, error) {
	repo, err := newSQLiteRepositoryWithDB(db)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
