package dialect

import (
	"fmt"
	"strings"
)

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

// JSONExtractPath returns the SQL fragment to extract a JSON value nested
// under one or more keys. It exists because JSONExtract's Postgres branch
// does single-level extraction only: a dotted path like "question.id" passed
// straight to JSONExtract would work on SQLite's native dotted json_extract
// syntax but silently return NULL on Postgres, where col::jsonb->>'a.b'
// treats the whole string as one literal top-level key rather than a nested
// traversal. Postgres needs a chained -> for every segment but the last,
// which gets ->> so the final result comes back as text like JSONExtract.
//
//	SQLite:   json_extract(col, '$.a.b')
//	Postgres: col::jsonb->'a'->>'b'
func JSONExtractPath(driver, col string, segments ...string) string {
	if len(segments) == 1 {
		return JSONExtract(driver, col, segments[0])
	}
	if IsPostgres(driver) {
		var b strings.Builder
		b.WriteString(col)
		b.WriteString("::jsonb")
		for _, seg := range segments[:len(segments)-1] {
			fmt.Fprintf(&b, "->'%s'", seg)
		}
		fmt.Fprintf(&b, "->>'%s'", segments[len(segments)-1])
		return b.String()
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", col, strings.Join(segments, "."))
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

// ExcludeTruthyMetadataPredicate returns a WHERE-clause fragment excluding
// rows whose col JSON has a truthy boolean value at key. SQLite's
// json_extract maps a JSON boolean to an integer (1/0); Postgres's ->>
// operator returns jsonb through text, so a boolean there reads back as the
// string "true"/"false" — the two dialects need different truthy
// comparisons for the same semantic check.
//
//	SQLite:   json_extract(col, '$.key') IS NOT 1
//	Postgres: COALESCE(col::jsonb->>'key', '') NOT IN ('true', '1')
func ExcludeTruthyMetadataPredicate(driver, col, key string) string {
	if IsPostgres(driver) {
		// Repository writes always marshal metadata as JSON; dirty Postgres
		// rows with malformed JSON should fail loudly instead of being
		// silently skipped.
		return fmt.Sprintf("COALESCE(%s, '') NOT IN ('true', '1')", JSONExtract(driver, col, key))
	}
	return fmt.Sprintf("%s IS NOT 1", JSONExtract(driver, col, key))
}

// ExcludeConfigModePredicate returns a WHERE-clause fragment excluding rows
// whose col JSON has a truthy "config_mode" key — office config-mode tasks
// are internal bookkeeping, not user-visible work items, so every task-scoped
// read that should match kandev's normal task-list semantics (the Host data
// API's Tasks/Sessions/CodeStats readers, the task list/search endpoints)
// applies this against the tasks table's metadata column.
func ExcludeConfigModePredicate(driver, col string) string {
	return ExcludeTruthyMetadataPredicate(driver, col, "config_mode")
}
