//go:build !cgo

package db

import (
	"context"
	"database/sql"
	"errors"
)

// BackupSQLite is unavailable in helper binaries built without SQLite support.
func BackupSQLite(_ context.Context, _ *sql.DB, _ string) error {
	return errors.New("SQLite backup requires CGO support")
}
