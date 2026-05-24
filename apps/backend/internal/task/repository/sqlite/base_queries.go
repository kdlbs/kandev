// Package sqlite provides SQLite-based repository implementations.
package sqlite

// sqlLimitClause is the SQL fragment appended to dynamic queries when a row
// limit is requested. Shared across plan.go, document.go, and message.go.
const sqlLimitClause = " LIMIT ?"
