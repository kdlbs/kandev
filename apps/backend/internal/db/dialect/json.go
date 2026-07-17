package dialect

import "fmt"

// JSONExtract returns the SQL fragment to extract a JSON value.
//
//	SQLite:   json_extract(col, '$.path')
//	Postgres: col::jsonb->>'path'
func JSONExtract(driver, col, path string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("%s::jsonb->>'%s'", col, path)
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", col, path)
}

// JSONExtractIsNotNull returns the SQL fragment to check that a JSON path is not null.
//
//	SQLite:   json_extract(col, '$.path') IS NOT NULL
//	Postgres: col::jsonb->>'path' IS NOT NULL
func JSONExtractIsNotNull(driver, col, path string) string {
	return JSONExtract(driver, col, path) + " IS NOT NULL"
}

// JSONSet returns the SQL fragment to set a JSON value.
//
//	SQLite:   json_set(col, '$.path', 'value')
//	Postgres: jsonb_set(col::jsonb, '{path}', '"value"')::text
func JSONSet(driver, col, path, value string) string {
	if IsPostgres(driver) {
		return fmt.Sprintf("jsonb_set(%s::jsonb, '{%s}', '\"%s\"')::text", col, path, value)
	}
	return fmt.Sprintf("json_set(%s, '$.%s', '%s')", col, path, value)
}

// ExcludeConfigModePredicate returns a WHERE-clause fragment excluding rows
// whose col JSON has a truthy "config_mode" key — office config-mode tasks
// are internal bookkeeping, not user-visible work items, so every task-scoped
// read that should match kandev's normal task-list semantics (the Host data
// API's Tasks/Sessions/CodeStats readers, the task list/search endpoints)
// applies this against the tasks table's metadata column.
//
//	SQLite:   json_extract(col, '$.config_mode') IS NOT 1
//	Postgres: COALESCE(col::jsonb->>'config_mode', '') NOT IN ('true', '1')
func ExcludeConfigModePredicate(driver, col string) string {
	if IsPostgres(driver) {
		// Repository writes always marshal metadata as JSON; dirty Postgres
		// rows with malformed JSON should fail loudly instead of being
		// silently skipped.
		return fmt.Sprintf("COALESCE(%s, '') NOT IN ('true', '1')", JSONExtract(driver, col, "config_mode"))
	}
	return fmt.Sprintf("%s IS NOT 1", JSONExtract(driver, col, "config_mode"))
}
