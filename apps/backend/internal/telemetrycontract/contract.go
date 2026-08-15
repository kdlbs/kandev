// Package telemetrycontract publishes the activation point of each telemetry
// collection contract this binary writes. The downstream analytics extract is
// a point-in-time snapshot with no schema versioning, so a column or table
// whose meaning changes mid-series is silently discontinuous unless its
// activation point is readable from the snapshot itself.
package telemetrycontract

// Contract describes one telemetry collection contract whose activation point
// and liveness must be readable from the database snapshot itself.
type Contract struct {
	// Key identifies the contract, e.g. "turn.workflow_step_id_at_start".
	Key string
	// Version starts at 1. A bump appends a new activation row rather than
	// overwriting the first.
	Version int
	// ExistsQuery is a SELECT returning exactly one row/column that is
	// non-NULL when the contract's backing objects exist.
	ExistsQuery string
	// StatsQuery is a SELECT returning (count, max_occurred_at); the second
	// column may be NULL when there are no rows yet.
	StatsQuery string
}

// registry is the closed set of contracts this binary writes.
//
// ExistsQuery is deliberately dialect-neutral: "SELECT 1 FROM <table> LIMIT 1"
// runs identically on SQLite and Postgres. A missing table surfaces as a query
// error (classified with db.IsMissingTableError), and an existing-but-empty
// table surfaces as sql.ErrNoRows — both are handled by the Store, which is
// what lets one query string answer the existence question on either dialect.
var registry = []Contract{
	{
		Key:         "turn.workflow_step_id_at_start",
		Version:     1,
		ExistsQuery: `SELECT 1 FROM task_session_turns LIMIT 1`,
		StatsQuery: `SELECT COUNT(*), MAX(started_at) FROM task_session_turns
			WHERE metadata LIKE '%"workflow_step_id_at_start"%'`,
	},
	{
		Key:         "task_step_transitions",
		Version:     1,
		ExistsQuery: `SELECT 1 FROM task_step_transitions LIMIT 1`,
		StatsQuery:  `SELECT COUNT(*), MAX(occurred_at) FROM task_step_transitions`,
	},
}

// Registry returns the closed set of contracts this binary writes.
func Registry() []Contract {
	return registry
}
