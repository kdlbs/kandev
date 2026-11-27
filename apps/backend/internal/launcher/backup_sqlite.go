package launcher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/db"
)

var copyProductionSnapshotFn = copyProductionSnapshot

func openProductionSnapshotReader(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := "file:" + db.EscapeSQLiteURIPath(filepath.ToSlash(absolute))
	reader, err := sql.Open("sqlite3", uri+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	reader.SetMaxOpenConns(1)
	return reader, nil
}

func copyProductionSnapshot(ctx context.Context, source, staged string) error {
	reader, err := openProductionSnapshotReader(source)
	if err != nil {
		return err
	}
	copyErr := db.BackupSQLite(ctx, reader, staged)
	return errors.Join(copyErr, reader.Close())
}

func verifyProductionSnapshot(ctx context.Context, path string) (err error) {
	reader, err := openProductionSnapshotReader(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	rows, err := reader.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	var count int
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return errors.New("snapshot integrity check failed")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("snapshot integrity check returned %d results", count)
	}
	return nil
}
