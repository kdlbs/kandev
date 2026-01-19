package store

import "database/sql"

// Provide creates the SQLite notifications store using the shared database connection.
func Provide(db *sql.DB) (*SQLiteRepository, func() error, error) {
	repo, err := NewSQLiteRepositoryWithDB(db)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}
