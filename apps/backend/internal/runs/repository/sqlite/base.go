// Package sqlite provides SQLite-based repository operations for the
// runs queue (renamed from office_runs in Phase 3 of
// task-model-unification). This package was lifted out of the office
// repository so the workflow engine and other callers can manage the
// runs queue without depending on the office domain.
package sqlite

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Repository provides SQLite-based runs queue storage. It holds
// separate writer and reader handles. The schema (CREATE TABLE,
// indexes, ALTER migrations) is owned by the office repository's
// init path; this struct only exposes data-access methods.
type Repository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader

	// claimLimits backs REQ-OFFICE-LAUNCH-SAFETY-001/005's claim-time
	// ceilings and budgets. See ClaimSafetyLimits and SetClaimSafetyLimits.
	claimLimits ClaimSafetyLimits

	// gateFailureThreshold backs REQ-OFFICE-BACKPRESSURE-003's durable
	// escalation record. See RecordGateOutcome and SetGateFailureThreshold.
	gateFailureThreshold int

	// log is optional (nil in most tests, via SetLogger otherwise) so
	// NewWithDB's signature stays unchanged for the many existing call
	// sites; ClaimNextEligibleRun's deferral-attribution log
	// (AC-OFFICE-BACKPRESSURE-003.7) is skipped when it is nil.
	log *logger.Logger
}

// SetLogger wires a logger for this repository's structured log
// emission (currently: ClaimNextEligibleRun's deferral-attribution
// entry, AC-OFFICE-BACKPRESSURE-003.7). Optional — a nil or never-set
// logger simply skips that log entry; the underlying counters and
// durable records still update.
func (r *Repository) SetLogger(log *logger.Logger) {
	r.log = log
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
