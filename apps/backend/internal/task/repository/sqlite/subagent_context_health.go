// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

// These three functions are the package-private health surface for AC-28/AC-29.
// No production endpoint, scheduled check, or alerting integration is wired to
// them in this build — a deployment that wants one wires it separately. Tests
// exercise them on both dialects (SR44 / AC-28 scope note). They must live in
// a regular .go file (not _test.go) because a future caller will be production
// code that needs a real DB connection, not testing.TB.

// subagentContextTimeLiteral renders t as a single-quoted RFC3339Nano SQL
// string literal. Every call site here passes a Go time.Time the caller
// controls (never request/caller input), so inlining is not a SQL-injection
// surface — the same convention subagentContextActivationKeyInsertSQL uses.
func subagentContextTimeLiteral(t time.Time) string {
	return "'" + t.UTC().Format(time.RFC3339Nano) + "'"
}

// subagentContextHealthCount runs a query whose single result column is a
// COUNT(*) and returns it as an int.
func (r *Repository) subagentContextHealthCount(query string) (int, error) {
	var count int
	if err := r.ro.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("subagent context health query: %w", err)
	}
	return count, nil
}

// subagentContextShortfall answers AC-28's SHORTFALL anti-join: distinct
// (task_session_id, tool_call_id) keys that AC-2/AC-21a expect a subagent
// context row for, among messages at or after since, that have no matching
// row in task_session_subagents. This is the direction AC-29 requires to
// diverge (become non-zero) the moment the writer stops while message writes
// continue, and the only direction of AC-28's comparison that alarms.
//
// Every timestamp comparison goes through subagentPreciseTimestamp (AC-24e):
// a raw string comparison between the RFC3339Nano since value and this
// repository's own stored TIMESTAMP form silently snaps to midnight of
// since's date. Every metadata reference goes through jsonKey/jsonColumn
// (AC-23b), which normalizes NULL/”/'null' to '{}' so one legacy row with
// blank metadata cannot abort the whole comparison on either dialect.
func (r *Repository) subagentContextShortfall(since time.Time) (int, error) {
	postgres := dialect.IsPostgres(r.db.DriverName())
	sinceExpr := subagentPreciseTimestamp(postgres, subagentContextTimeLiteral(since))
	toolCallID := jsonKey(postgres, "m.metadata", "tool_call_id")
	kind := jsonKey(postgres, "m.metadata", "normalized.kind")

	query := `
		SELECT COUNT(*) FROM (
			SELECT m.task_session_id, ` + toolCallID + ` AS tool_call_id
			FROM task_session_messages m
			WHERE ` + kind + ` = 'subagent_task'
				AND ` + subagentPreciseTimestamp(postgres, "m.created_at") + ` >= ` + sinceExpr + `
				AND COALESCE(m.task_session_id, '') <> ''
				AND COALESCE(m.task_id, '') <> ''
				AND COALESCE(` + toolCallID + `, '') <> ''
			GROUP BY 1, 2
			EXCEPT
			SELECT s.task_session_id, s.tool_call_id
			FROM task_session_subagents s
			WHERE ` + subagentPreciseTimestamp(postgres, "s.observed_at") + ` >= ` + sinceExpr + `
		) AS shortfall
	`
	return r.subagentContextHealthCount(query)
}

// subagentContextExcess answers AC-28's EXCESS anti-join: distinct
// (task_session_id, tool_call_id) keys present in task_session_subagents at
// or after since with no accounting message. Attributed (a message-write
// failure with a successful context upsert, or a message straddling the
// activation boundary — see AC-1a) and does NOT alarm; it can never mask a
// shortfall because it is counted in the opposite direction.
//
// Deliberately does NOT carry the shortfall side's task_session_id/task_id/
// tool_call_id non-empty predicates: the excess side asks "is there any
// message that accounts for this row", so it must subtract the widest
// message set available, not the AC-2/AC-21a-filtered one. Filtering it here
// would misreport excess for a row whose message legitimately has some other
// column empty.
func (r *Repository) subagentContextExcess(since time.Time) (int, error) {
	postgres := dialect.IsPostgres(r.db.DriverName())
	sinceExpr := subagentPreciseTimestamp(postgres, subagentContextTimeLiteral(since))
	toolCallID := jsonKey(postgres, "m.metadata", "tool_call_id")
	kind := jsonKey(postgres, "m.metadata", "normalized.kind")

	query := `
		SELECT COUNT(*) FROM (
			SELECT s.task_session_id, s.tool_call_id
			FROM task_session_subagents s
			WHERE ` + subagentPreciseTimestamp(postgres, "s.observed_at") + ` >= ` + sinceExpr + `
			GROUP BY 1, 2
			EXCEPT
			SELECT m.task_session_id, ` + toolCallID + `
			FROM task_session_messages m
			WHERE ` + kind + ` = 'subagent_task'
				AND ` + subagentPreciseTimestamp(postgres, "m.created_at") + ` >= ` + sinceExpr + `
		) AS excess
	`
	return r.subagentContextHealthCount(query)
}

// subagentContextMaxObservedSince answers AC-29's attribution half: the
// newest observed_at among rows at or after since (the same since and the
// same subagentPreciseTimestamp comparison the shortfall/excess queries
// use), or the zero time when no such row exists. A caller MUST treat the
// zero-time/false result as "the writer has produced nothing since this
// installation activated" — the strongest available divergence signal, not
// an absence of information — never as "no divergence", "unknown", or a
// healthy state (AC-29 revision 4). Scoping to since is required: an
// unscoped MAX(observed_at) reports the newest of whatever exists, which on
// an installation whose only rows are backfilled or legacy reports an
// instant that never established live-writer health at all.
func (r *Repository) subagentContextMaxObservedSince(since time.Time) (time.Time, bool, error) {
	postgres := dialect.IsPostgres(r.db.DriverName())
	sinceExpr := subagentPreciseTimestamp(postgres, subagentContextTimeLiteral(since))

	query := `
		SELECT MAX(s.observed_at)
		FROM task_session_subagents s
		WHERE ` + subagentPreciseTimestamp(postgres, "s.observed_at") + ` >= ` + sinceExpr + `
	`
	// SQLite's driver only auto-converts a column value to time.Time when it
	// can see the declared column type; MAX() loses that (see
	// parseLegacyTimestamp's comment), so the scan target must be a string on
	// that dialect. PostgreSQL's driver has no such gap.
	if postgres {
		var maxObserved sql.NullTime
		if err := r.ro.QueryRow(query).Scan(&maxObserved); err != nil {
			return time.Time{}, false, fmt.Errorf("subagent context max observed since: %w", err)
		}
		if !maxObserved.Valid {
			return time.Time{}, false, nil
		}
		return maxObserved.Time.UTC(), true, nil
	}
	var maxObservedRaw sql.NullString
	if err := r.ro.QueryRow(query).Scan(&maxObservedRaw); err != nil {
		return time.Time{}, false, fmt.Errorf("subagent context max observed since: %w", err)
	}
	if !maxObservedRaw.Valid || maxObservedRaw.String == "" {
		return time.Time{}, false, nil
	}
	parsed := parseLegacyTimestamp(maxObservedRaw.String)
	if parsed.IsZero() {
		return time.Time{}, false, fmt.Errorf("subagent context max observed since: unparsable timestamp %q", maxObservedRaw.String)
	}
	return parsed, true, nil
}
