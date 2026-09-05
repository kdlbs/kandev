// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// sqlLimitClause is the SQL fragment appended to dynamic queries when a row
// limit is requested. Shared across plan.go, document.go, and message.go.
const sqlLimitClause = " LIMIT ?"

// sqliteMaxHostParams is the safe upper bound on placeholders in a single
// statement. SQLite's compile-time default (SQLITE_MAX_VARIABLE_NUMBER) is
// 999 on older builds and 32766 on newer ones; we chunk well below the
// lower bound so batched IN-clause queries stay portable across builds.
const sqliteMaxHostParams = 500

// worktree repo status values on task_environment_repos. Mirrors the worktree
// package's StatusActive/StatusMerged/StatusDeleted lifecycle.
const (
	worktreeRepoStatusActive  = "active"
	worktreeRepoStatusMerged  = "merged"
	worktreeRepoStatusDeleted = "deleted"
)

// buildInPlaceholders returns a comma-separated "?,?,?" placeholder string and
// the matching args slice for an IN (...) clause over the given string IDs.
// Caller is responsible for splitting overly large input via chunkIDs.
func buildInPlaceholders(ids []string) (string, []interface{}) {
	if len(ids) == 0 {
		return "", nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders, args
}

// listLimitedCandidates runs query against ro (appending sqlLimitClause and
// limit as a trailing bound parameter when limit > 0), scanning each row
// with scanRow. It centralizes the query/scan-loop shape shared by the
// read-only maintenance-candidate listers (duplicate git snapshots,
// orphaned message payloads, ...) so each lister only supplies its SQL and
// per-row scan logic.
func listLimitedCandidates[T any](ctx context.Context, ro *sqlx.DB, query string, limit int, errLabel string, scanRow func(*sql.Rows) (T, error)) ([]T, error) {
	args := []interface{}{}
	if limit > 0 {
		query += sqlLimitClause
		args = append(args, limit)
	}

	rows, err := ro.QueryContext(ctx, ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", errLabel, err)
	}
	defer func() { _ = rows.Close() }()

	var out []T
	for rows.Next() {
		item, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", errLabel, err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", errLabel, err)
	}
	return out, nil
}

// chunkIDs splits ids into sub-slices of at most size entries. Useful when
// callers want to keep IN-clause queries below sqliteMaxHostParams. An empty
// input returns an empty outer slice (NOT a single nil chunk) so callers can
// range over the result safely — a single-nil-chunk would produce an empty
// `IN ()` clause and a SQL syntax error when fed to buildInPlaceholders.
func chunkIDs(ids []string, size int) [][]string {
	if len(ids) == 0 {
		return nil
	}
	if size <= 0 || len(ids) <= size {
		return [][]string{ids}
	}
	chunks := make([][]string, 0, (len(ids)+size-1)/size)
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[i:end])
	}
	return chunks
}
