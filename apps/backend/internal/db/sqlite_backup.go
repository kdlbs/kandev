//go:build cgo

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-sqlite3"
)

// BackupSQLite copies a consistent image without compacting it. The destination
// must be new; callers own integrity verification and publication of the copy.
func BackupSQLite(ctx context.Context, source *sql.DB, path string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
			_ = os.Remove(path + "-journal")
		}
	}()
	if err = file.Close(); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uri := "file:" + EscapeSQLiteURIPath(filepath.ToSlash(absolute))
	destination, err := sql.Open("sqlite3", uri+"?mode=rw&_journal_mode=DELETE&_synchronous=FULL&_busy_timeout=0")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, destination.Close()) }()
	destination.SetMaxOpenConns(1)
	return copySQLiteConnections(ctx, source, destination)
}

func copySQLiteConnections(ctx context.Context, source, destination *sql.DB) error {
	src, err := source.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := destination.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	var pageSize int
	if err := src.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return err
	}
	if pageSize <= 0 {
		return fmt.Errorf("invalid SQLite page size: %d", pageSize)
	}
	return src.Raw(func(sourceDriver any) error {
		sourceSQLite, ok := sourceDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return errors.New("backup source is not a SQLite connection")
		}
		return dst.Raw(func(destinationDriver any) error {
			destinationSQLite, ok := destinationDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.New("backup destination is not a SQLite connection")
			}
			return copySQLitePages(ctx, sourceSQLite, destinationSQLite, max(1, (4<<20)/pageSize))
		})
	})
}

func copySQLitePages(ctx context.Context, source, destination *sqlite3.SQLiteConn, pages int) (err error) {
	backup, err := destination.Backup("main", source, "main")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backup.Finish()) }()
	lastProgress := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining, total := backup.Remaining(), backup.PageCount()
		done, err := backup.Step(pages)
		if err != nil {
			return err
		}
		if done {
			return ctx.Err()
		}
		// BUSY/LOCKED return false,nil too. Only wait when the copy did not advance.
		if backup.Remaining() >= remaining && backup.PageCount() == total {
			if time.Since(lastProgress) >= defaultBusyTimeout {
				return fmt.Errorf("SQLite backup made no progress for %s", defaultBusyTimeout)
			}
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		} else {
			lastProgress = time.Now()
		}
	}
}
