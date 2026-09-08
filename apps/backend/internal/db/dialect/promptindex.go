package dialect

import "fmt"

// NormalizedMicrosecond returns a SQL expression evaluating to the UTC
// microsecond-ordering key of a message timestamp: exactly
// `YYYY-MM-DD HH:MM:SS.ffffff` (space separator, zero-padded 6-digit fraction,
// no zone suffix). It is the single source of truth for the prompt-ordering
// key used by the paginated message window `ORDER BY`, the outer cursor
// predicate/ordering, and the single-row correlated count, so SQLite,
// PostgreSQL, and the frontend's `epochNanoseconds / 1000n` UTC floor all
// agree on row order.
//
// On PostgreSQL the column is a naive TIMESTAMP storing UTC wall-clock at
// microsecond precision, so the column itself is the key (an identity/cast).
// On SQLite the column is TEXT written by mattn/go-sqlite3 as
// `YYYY-MM-DD HH:MM:SS.fffffffff+00:00` (Go's `.999999999` layout trims
// trailing zeros; the offset is always present, and exact-second values have
// no fraction at all). datetime() parses the FULL stored string — wall clock,
// fraction, and offset — and converts to UTC at second precision; the fraction
// is carried separately (right-padded to 9 digits, truncated to 6) because
// datetime() and strftime('%f') drop sub-second precision and julianday lacks
// float precision for microseconds. The fraction dot, when present, sits at
// fixed position 20 in both the driver format and RFC3339 forms; the digit
// run starts at position 21 and ends at the first `+`/`-`/`Z`. Without a dot
// the run is empty (exact-second and offset-only forms).
func NormalizedMicrosecond(driver, expr string) string {
	if IsPostgres(driver) {
		return expr
	}
	rest := "substr(" + expr + ", 21)"
	digitRun := "CASE " +
		"WHEN instr(" + rest + ", '+') > 0 THEN substr(" + expr + ", 21, instr(" + rest + ", '+') - 1) " +
		"WHEN instr(" + rest + ", '-') > 0 THEN substr(" + expr + ", 21, instr(" + rest + ", '-') - 1) " +
		"WHEN instr(" + rest + ", 'Z') > 0 THEN substr(" + expr + ", 21, instr(" + rest + ", 'Z') - 1) " +
		"ELSE " + rest + " END"
	return fmt.Sprintf(
		"substr(datetime(%s), 1, 19) || '.' || substr((CASE WHEN substr(%s, 20, 1) = '.' THEN %s ELSE '' END) || '000000000', 1, 6)",
		expr, expr, digitRun,
	)
}

// PromptOrderIndexDDL returns the additive expression index that turns the
// prompt-ordering window pass into an index-only scan with no per-page sort
// and serves the outer cursor predicate and ORDER BY. The indexed expression
// textually equals NormalizedMicrosecond output, so the query expression and
// the index cannot drift. CREATE INDEX IF NOT EXISTS is replay-safe on both
// dialects.
func PromptOrderIndexDDL(driver, indexName, table string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s(task_session_id, created_at, id)",
			indexName, table,
		)
	}
	return fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s(task_session_id, (%s), id)",
		indexName, table, NormalizedMicrosecond(driver, "created_at"),
	)
}

// PromptUserOrderIndexDDL returns an index for filtered prompt pages. The
// author_type prefix lets the database skip interleaved agent and tool rows
// before it applies the normalized timestamp ordering expression.
func PromptUserOrderIndexDDL(driver, indexName, table string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s(task_session_id, author_type, created_at, id)",
			indexName, table,
		)
	}
	return fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s(task_session_id, author_type, (%s), id)",
		indexName, table, NormalizedMicrosecond(driver, "created_at"),
	)
}
