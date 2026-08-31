package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// officeTaskSessionIndexName names the partial unique index that would
// enforce at most one "live" task_sessions row per (task_id,
// agent_profile_id) pair. It is not wired into base_schema.go yet — see
// ErrOfficeSessionRaceConflict's doc comment in errors.go for why — but
// naming it here keeps isOfficeTaskSessionUniqueViolation attributable to
// this constraint specifically once it lands. Mirrors the (workspace_id,
// external_id) classification in task_external_id.go.
const officeTaskSessionIndexName = "uniq_office_task_session"

// sqliteOfficeTaskSessionViolationMessage is the substring go-sqlite3 puts in
// a UNIQUE-constraint error for this specific index. SQLite's driver exposes
// no typed access to the violated index's name, only the column list of the
// failing index ("UNIQUE constraint failed: task_sessions.task_id,
// task_sessions.agent_profile_id"); that column pair is unique to
// uniq_office_task_session in this table (the only other unique constraint is
// the id primary key), so matching it attributes the violation correctly — a
// bare "UNIQUE constraint failed" match would also fire on the primary key.
//
// SQLite lists the columns in the index's own DEFINITION order, not table
// column order (verified directly: an index declared ON t(b, a) reports
// "t.b, t.a"). This message is therefore contingent on the eventual
// uniq_office_task_session DDL declaring its columns as
// (task_id, agent_profile_id) in that order — flipping the order in the
// migration silently deadens this match again, with the code, tests, and
// comments all still reading correct.
const sqliteOfficeTaskSessionViolationMessage = "UNIQUE constraint failed: task_sessions.task_id, task_sessions.agent_profile_id"

// isOfficeTaskSessionUniqueViolation reports whether err is a violation of
// uniq_office_task_session specifically, not any unique violation. On
// PostgreSQL it inspects the typed pgconn.PgError's constraint name; on
// SQLite (no typed access to the constraint name) it matches the column-list
// message documented above. A plain strings.Contains(err.Error(), the index
// name) is not sufficient by itself: Postgres embeds the constraint name in
// its error text, but SQLite never does, so that check alone would leave
// this classification dead on SQLite even with the index in place.
func isOfficeTaskSessionUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == officeTaskSessionIndexName
	}
	return strings.Contains(err.Error(), sqliteOfficeTaskSessionViolationMessage)
}
