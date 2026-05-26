// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

// Repository provides SQLite-based task storage operations.
type Repository struct {
	db      *sqlx.DB // writer
	ro      *sqlx.DB // reader (read-only pool)
	ownsDB  bool
	log     *logger.Logger
	migrate *db.MigrateLogger
}

// NewWithDB creates a new SQLite repository with an existing database connection (shared ownership).
func NewWithDB(writer, reader *sqlx.DB, log *logger.Logger) (*Repository, error) {
	return newRepository(writer, reader, log, false)
}

func newRepository(writer, reader *sqlx.DB, log *logger.Logger, ownsDB bool) (*Repository, error) {
	repo := &Repository{
		db:      writer,
		ro:      reader,
		ownsDB:  ownsDB,
		log:     log,
		migrate: db.NewMigrateLogger(writer, log),
	}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *Repository) DB() *sql.DB {
	return r.db.DB
}
