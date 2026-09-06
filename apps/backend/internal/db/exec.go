package db

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// Rebinder is implemented by sqlx.DB and sqlx.Tx. Queries remain in the
// common question-mark form until they reach the final database boundary.
type Rebinder interface {
	Rebind(string) string
}

// ContextExecer is the final writer boundary used by ExecContext.
type ContextExecer interface {
	Rebinder
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Bind expands sqlx.In arguments and then rebinds placeholders exactly once.
// Callers must pass the returned query directly to the same database or
// transaction boundary; do not call Rebind on it again.
func Bind(rebinder Rebinder, query string, args ...any) (string, []any, error) {
	expanded, expandedArgs, err := sqlx.In(query, args...)
	if err != nil {
		return "", nil, err
	}
	return rebinder.Rebind(expanded), expandedArgs, nil
}

// ExecContext expands, rebinds, and executes a query at its final boundary.
func ExecContext(ctx context.Context, execer ContextExecer, query string, args ...any) (sql.Result, error) {
	bound, boundArgs, err := Bind(execer, query, args...)
	if err != nil {
		return nil, err
	}
	return execer.ExecContext(ctx, bound, boundArgs...)
}
