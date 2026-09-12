package dialect

import "fmt"

// OrderedIDConcat returns the SQL fragment for a scalar subquery that
// comma-joins the `id` column of `SELECT id FROM tasks WHERE <where>`, in
// ascending id order. The two dialects place the ordering differently:
// SQLite's GROUP_CONCAT has no ORDER BY of its own, so ordering comes from
// an ORDER BY inside the subquery; PostgreSQL's string_agg takes an ORDER BY
// inside the aggregate call itself. Writing one and assuming the other
// matches is how the two dialects silently diverge on row order — see
// docs/specs/office/system-design/parent-wake-wave-identity.md, "Backstop
// admission".
//
// where is the WHERE clause body (no leading "WHERE", no trailing
// "ORDER BY") of the `tasks` subquery — the caller supplies the predicate,
// this helper only owns the aggregate/order-by placement split.
//
//	SQLite:   (SELECT GROUP_CONCAT(w.id, ',') FROM (SELECT id FROM tasks WHERE <where> ORDER BY id) w)
//	Postgres: (SELECT string_agg(w.id, ',' ORDER BY w.id) FROM (SELECT id FROM tasks WHERE <where>) w)
func OrderedIDConcat(driver, where string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf(
			"(SELECT string_agg(w.id, ',' ORDER BY w.id) FROM (SELECT id FROM tasks WHERE %s) w)", where)
	}
	return fmt.Sprintf(
		"(SELECT GROUP_CONCAT(w.id, ',') FROM (SELECT id FROM tasks WHERE %s ORDER BY id) w)", where)
}

// OrderedPairConcat is OrderedIDConcat's sibling for a two-column
// "id:state" key: it comma-joins `id || ':' || state` from
// `SELECT id, state FROM tasks WHERE <where>`, in ascending id order, using
// the same SQLite-subquery-vs-Postgres-aggregate ordering split. Callers
// that also format the same key in Go (e.g. by concatenating id and state
// per row) must produce a byte-identical string, since the two forms are
// compared directly.
//
//	SQLite:   (SELECT GROUP_CONCAT(w.id || ':' || w.state, ',') FROM (SELECT id, state FROM tasks WHERE <where> ORDER BY id) w)
//	Postgres: (SELECT string_agg(w.id || ':' || w.state, ',' ORDER BY w.id) FROM (SELECT id, state FROM tasks WHERE <where>) w)
func OrderedPairConcat(driver, where string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf(
			"(SELECT string_agg(w.id || ':' || w.state, ',' ORDER BY w.id) FROM (SELECT id, state FROM tasks WHERE %s) w)", where)
	}
	return fmt.Sprintf(
		"(SELECT GROUP_CONCAT(w.id || ':' || w.state, ',') FROM (SELECT id, state FROM tasks WHERE %s ORDER BY id) w)", where)
}
