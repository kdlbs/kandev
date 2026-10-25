package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDuplicateUsageEvent is returned when an insert into task_usage_events
// collides with an existing row's usage_event_id. The real duplicate source
// is the same underlying usage frame reaching the ledger writer twice (e.g.
// a reconnecting WS client replaying a buffered stream event), not
// double-spend, so callers treat this as an idempotent no-op (AC-13, AC-14,
// AC-32) rather than a failure. Mirrors
// internal/office/repository/sqlite/costs.go's ErrDuplicateUsageEvent.
var ErrDuplicateUsageEvent = errors.New("duplicate usage_event_id")

// usageEventIDIndexName is the named unique index enforcing at most one
// task_usage_events row per usage_event_id
// (docs/specs/task-cost-ledger/spec.md, "task_usage_events (new table)").
// Naming it explicitly here keeps isUsageEventUniqueViolation attributable
// to this constraint specifically, not to the table's primary key or any
// other unique constraint.
const usageEventIDIndexName = "uniq_task_usage_events_usage_event_id"

// sqliteUsageEventIDViolationMessage is the substring go-sqlite3 puts in a
// UNIQUE-constraint error for this index. SQLite exposes no typed access to
// the violated index's name, only the column list of the failing index
// ("UNIQUE constraint failed: task_usage_events.usage_event_id"); that
// column is unique to uniq_task_usage_events_usage_event_id in this table,
// so matching it attributes the violation correctly - a bare "UNIQUE
// constraint failed" match would also fire on the table's primary key.
const sqliteUsageEventIDViolationMessage = "UNIQUE constraint failed: task_usage_events.usage_event_id"

// isUsageEventUniqueViolation reports whether err is a violation of
// uniq_task_usage_events_usage_event_id specifically, not any unique
// violation (AC-32). On PostgreSQL it inspects the typed pgconn.PgError's
// constraint name; on SQLite (no typed access to the constraint name) it
// matches the column-list message documented above. Mirrors
// isExternalIDUniqueViolation (task_external_id.go) and
// internal/office/repository/sqlite/costs.go's isUsageEventUniqueViolation.
func isUsageEventUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == usageEventIDIndexName
	}
	return strings.Contains(err.Error(), sqliteUsageEventIDViolationMessage)
}
