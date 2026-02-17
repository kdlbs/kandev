package dialect

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// InsertReturningID executes an INSERT and returns the auto-generated ID.
//
//	Postgres: appends RETURNING id and scans the result.
//	SQLite:   uses LastInsertId() from the exec result.
func InsertReturningID(ctx context.Context, db *sqlx.DB, query string, args ...any) (int64, error) {
	if IsPostgres(db.DriverName()) {
		var id int64
		err := db.QueryRowContext(ctx, db.Rebind(query+" RETURNING id"), args...).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("insert returning id: %w", err)
		}
		return id, nil
	}

	result, err := db.ExecContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
