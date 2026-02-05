// Package sqlite provides SQLite-based analytics repository implementations.
package sqlite

import "database/sql"

// Repository provides SQLite-based analytics operations.
type Repository struct {
	db *sql.DB
}

// NewWithDB creates a new analytics repository with an existing database connection.
func NewWithDB(dbConn *sql.DB) (*Repository, error) {
	return &Repository{db: dbConn}, nil
}

// Close is a no-op because this repository does not own the database connection.
func (r *Repository) Close() error {
	return nil
}
