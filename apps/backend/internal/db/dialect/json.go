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
