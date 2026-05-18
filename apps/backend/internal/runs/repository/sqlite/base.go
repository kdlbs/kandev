// Package sqlite provides SQLite-based repository operations for the
// runs queue (renamed from office_runs in Phase 3 of
// task-model-unification). This package was lifted out of the office
// repository so the workflow engine and other callers can manage the
// runs queue without depending on the office domain.
package sqlite

import (
	"github.com/jmoiron/sqlx"
)

// Repository provides SQLite-based runs queue storage. It holds
// separate writer and reader handles. The schema (CREATE TABLE,
// indexes, ALTER migrations) is owned by the office repository's
// init path; this struct only exposes data-access methods.
type Repository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewWithDB creates a new runs repository with existing database
// connections. The runs / run_events tables are expected to already
// exist; the office repository's initSchema is responsible for
// creating them.
func NewWithDB(writer, reader *sqlx.DB) *Repository {
	return &Repository{db: writer, ro: reader}
}

// Writer returns the writer DB handle for callers that need to run
// joined queries against runs alongside their own tables.
func (r *Repository) Writer() *sqlx.DB { return r.db }

// Reader returns the reader DB handle for callers that need to run
// joined queries against runs alongside their own tables.
func (r *Repository) Reader() *sqlx.DB { return r.ro }
