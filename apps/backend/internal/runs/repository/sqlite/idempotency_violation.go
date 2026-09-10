package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// runIdempotencyIndexName is the unique index enforcing at most one runs row
// per non-null idempotency_key (internal/office/repository/sqlite/base.go,
// idx_run_idempotency).
const runIdempotencyIndexName = "idx_run_idempotency"

// sqliteRunIdempotencyViolationMessage is the substring go-sqlite3 puts in a
// UNIQUE-constraint error for this index. SQLite exposes no typed access to
// the violated index's name, only the column ("UNIQUE constraint failed:
// runs.idempotency_key"); that column is unique to idx_run_idempotency on
// this table, so matching it attributes the violation correctly rather than
// to the table's primary key.
const sqliteRunIdempotencyViolationMessage = "UNIQUE constraint failed: runs.idempotency_key"

// IsIdempotencyKeyUniqueViolation reports whether err is a violation of
// idx_run_idempotency specifically, not any unique violation. On PostgreSQL
// it inspects the typed pgconn.PgError's constraint name; on SQLite (no
// typed access to the constraint name) it matches the message documented
// above. Mirrors isExternalIDUniqueViolation
// (internal/task/repository/sqlite/task_external_id.go).
func IsIdempotencyKeyUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == runIdempotencyIndexName
	}
	return strings.Contains(err.Error(), sqliteRunIdempotencyViolationMessage)
}
