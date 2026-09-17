// Package sqlite provides SQLite-based repository operations for the
// runs queue (renamed from office_runs in Phase 3 of
// task-model-unification). This package was lifted out of the office
// repository so the workflow engine and other callers can manage the
// runs queue without depending on the office domain.
package sqlite

import (
	"github.com/jmoiron/sqlx"
)

// Repository provides core run queue storage over writer and reader handles.
type Repository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewWithDB creates a repository. Call Migrate before using a new database.
func NewWithDB(writer, reader *sqlx.DB) *Repository {
	return &Repository{db: writer, ro: reader}
}

// Writer returns the writer DB handle for callers that need to run
// joined queries against runs alongside their own tables.
func (r *Repository) Writer() *sqlx.DB { return r.db }

// Reader returns the reader DB handle for callers that need to run
// joined queries against runs alongside their own tables.
func (r *Repository) Reader() *sqlx.DB { return r.ro }
