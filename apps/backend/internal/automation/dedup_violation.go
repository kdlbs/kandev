package automation

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqliteRunDedupUniqueViolationMessage is the substring go-sqlite3 puts in a
// UNIQUE-constraint error for idx_automation_runs_dedup_unique
// (migrateRunDedupUniqueIndexSQL) — confirmed empirically via
// TestCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation, not assumed
// from another index's message shape. Mirrors
// internal/runs/repository/sqlite.IsWakeWaveUniqueViolation's composite-index
// convention: SQLite lists every participating column regardless of the
// index's WHERE clause.
const sqliteRunDedupUniqueViolationMessage = "UNIQUE constraint failed: automation_runs.automation_id, automation_runs.dedup_key"

// IsDedupKeyUniqueViolation reports whether err is a violation of
// idx_automation_runs_dedup_unique specifically, not any unique violation.
// On PostgreSQL it inspects the typed pgconn.PgError's constraint name,
// which for an index-backed unique violation is the index's own name; on
// SQLite (no typed access to the constraint name) it matches the message.
func IsDedupKeyUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "idx_automation_runs_dedup_unique"
	}
	return strings.Contains(err.Error(), sqliteRunDedupUniqueViolationMessage)
}
