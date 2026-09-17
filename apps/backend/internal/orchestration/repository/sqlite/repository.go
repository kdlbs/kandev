// Package sqlite owns workspace orchestrator registrations and role templates.
// It depends on core profile/workspace tables, not the Office feature.
package sqlite

import "github.com/jmoiron/sqlx"

type Repository struct{ db, ro *sqlx.DB }

func New(db, ro *sqlx.DB) *Repository {
	if ro == nil {
		ro = db
	}
	return &Repository{db: db, ro: ro}
}
