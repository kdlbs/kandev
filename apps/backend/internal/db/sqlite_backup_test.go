//go:build cgo

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mattn/go-sqlite3"
)

func TestPinSQLiteWALSnapshotDuringIncrementalBackup(t *testing.T) {
	fixture := newWALSnapshotFixture(t, filepath.Join(t.TempDir(), "source.db"))
	destinationPath := filepath.Join(t.TempDir(), "copy.db")
	destination := openBackupTestDB(t, destinationPath, "mode=rwc&_journal_mode=DELETE&_busy_timeout=0")
	target, err := destination.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	assertSQLiteAutoCommit(t, fixture.source, false)
	steps, writes := copySQLiteBackupWithWrites(t, fixture.source, target, fixture.writer)
	if steps < 2 || writes < 3 {
		t.Fatalf("backup used %d steps with %d independent commits", steps, writes)
	}
	copyReader := openBackupTestDB(t, destinationPath, "mode=ro")
	if copiedRows := countBackupTestRows(t, copyReader); copiedRows != 1 {
		t.Fatalf("snapshot contains %d rows, want its initial row only", copiedRows)
	}
	if err := fixture.release(); err != nil {
		t.Fatal(err)
	}
	if sourceRows := countBackupTestConnRows(t, fixture.source); sourceRows != writes+1 {
		t.Fatalf("reused source has %d rows, want latest committed count %d", sourceRows, writes+1)
	}
	assertSQLiteAutoCommit(t, fixture.source, true)
}

type walSnapshotFixture struct {
	writer  *sql.DB
	source  *sql.Conn
	release func() error
}

func newWALSnapshotFixture(t *testing.T, path string) walSnapshotFixture {
	t.Helper()
	ctx := t.Context()
	writer := openBackupTestDB(t, path, "mode=rwc&_busy_timeout=5000")
	if _, err := writer.ExecContext(ctx, `PRAGMA page_size=512`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `CREATE TABLE items (value INTEGER PRIMARY KEY, payload BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `INSERT INTO items VALUES (1, zeroblob(8192))`); err != nil {
		t.Fatal(err)
	}
	reader := openBackupTestDB(t, path, "mode=ro&_busy_timeout=5000")
	source, err := reader.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	release, err := pinSQLiteWALSnapshot(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if release == nil {
		t.Fatal("autocommit WAL source did not acquire a snapshot")
	}
	release = onceRelease(t, release)
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Error(err)
		}
	})
	return walSnapshotFixture{writer: writer, source: source, release: release}
}

func onceRelease(t *testing.T, release func() error) func() error {
	t.Helper()
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() { releaseErr = release() })
		return releaseErr
	}
}

func copySQLiteBackupWithWrites(t *testing.T, source, target *sql.Conn, writer *sql.DB) (int, int) {
	t.Helper()
	var backup *sqlite3.SQLiteBackup
	if err := target.Raw(func(targetDriver any) error {
		return source.Raw(func(sourceDriver any) error {
			var err error
			backup, err = targetDriver.(*sqlite3.SQLiteConn).Backup("main", sourceDriver.(*sqlite3.SQLiteConn), "main")
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	finished := false
	t.Cleanup(func() {
		if !finished {
			_ = backup.Finish()
		}
	})
	steps, writes, done := 0, 0, false
	for !done {
		var err error
		done, err = backup.Step(1)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		if done {
			break
		}
		for range 3 {
			if _, err := writer.ExecContext(t.Context(), `INSERT INTO items VALUES (?, zeroblob(100))`, writes+2); err != nil {
				t.Fatalf("writer commit %d: %v", writes+1, err)
			}
			writes++
		}
		if rows := countBackupTestConnRows(t, source); rows != 1 {
			t.Fatalf("pinned source has %d rows after writer commits, want 1", rows)
		}
	}
	if err := backup.Finish(); err != nil {
		t.Fatal(err)
	}
	finished = true
	return steps, writes
}

func countBackupTestRows(t *testing.T, database *sql.DB) int {
	t.Helper()
	var rows int
	if err := database.QueryRowContext(t.Context(), `SELECT count(*) FROM items`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func countBackupTestConnRows(t *testing.T, connection *sql.Conn) int {
	t.Helper()
	var rows int
	if err := connection.QueryRowContext(t.Context(), `SELECT count(*) FROM items`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertSQLiteAutoCommit(t *testing.T, connection *sql.Conn, want bool) {
	t.Helper()
	if err := connection.Raw(func(driver any) error {
		if got := driver.(*sqlite3.SQLiteConn).AutoCommit(); got != want {
			t.Fatalf("source autocommit is %t, want %t", got, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPinSQLiteWALSnapshotCleanupAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path := filepath.Join(t.TempDir(), "source.db")
	writer := openBackupTestDB(t, path, "mode=rwc&_journal_mode=WAL")
	if _, err := writer.ExecContext(ctx, `CREATE TABLE items (value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `INSERT INTO items VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	reader := openBackupTestDB(t, path, "mode=ro")
	source, err := reader.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	releaseSnapshot, err := pinSQLiteWALSnapshot(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if releaseSnapshot == nil {
		t.Fatal("autocommit WAL source did not acquire a snapshot")
	}
	releaseSnapshot = onceRelease(t, releaseSnapshot)
	t.Cleanup(func() {
		if err := releaseSnapshot(); err != nil {
			t.Error(err)
		}
	})
	if _, err := writer.ExecContext(ctx, `INSERT INTO items VALUES (2)`); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := releaseSnapshot(); err != nil {
		t.Fatal(err)
	}
	var sourceRows int
	if err := source.QueryRowContext(context.Background(), `SELECT count(*) FROM items`).Scan(&sourceRows); err != nil {
		t.Fatal(err)
	}
	if sourceRows != 2 {
		t.Fatalf("reused source has %d rows after cancellation, want latest committed count 2", sourceRows)
	}
	assertSQLiteAutoCommit(t, source, true)
}

func TestPinSQLiteWALSnapshotSchemaReadFailureRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	writer := openBackupTestDB(t, path, "mode=rwc&_journal_mode=WAL")
	if _, err := writer.Exec(`CREATE TABLE items (value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`INSERT INTO items VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	reader := openBackupTestDB(t, path, "mode=ro")
	source, err := reader.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })

	transactionStarted, schemaReadDenied := false, false
	if err := source.Raw(func(driver any) error {
		driver.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(op int, arg1, arg2, arg3 string) int {
			if op == sqlite3.SQLITE_TRANSACTION && arg1 == "BEGIN" {
				transactionStarted = true
			}
			if transactionStarted && op == sqlite3.SQLITE_READ && arg1 == "sqlite_schema" {
				schemaReadDenied = true
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if release, err := pinSQLiteWALSnapshot(t.Context(), source); err == nil || release != nil {
		t.Fatalf("snapshot setup returned release=%t, err=%v; want setup error", release != nil, err)
	}
	if !schemaReadDenied {
		t.Fatal("authorizer did not reject the snapshot-establishing schema read")
	}
	assertSQLiteAutoCommit(t, source, true)
	if err := source.Raw(func(driver any) error {
		driver.(*sqlite3.SQLiteConn).RegisterAuthorizer(nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`INSERT INTO items VALUES (2)`); err != nil {
		t.Fatal(err)
	}
	if rows := countBackupTestConnRows(t, source); rows != 2 {
		t.Fatalf("reused source has %d rows after setup failure, want latest committed count 2", rows)
	}
}

func TestPinSQLiteWALSnapshotLeavesCallerTransactionOwned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	database := openBackupTestDB(t, path, "mode=rwc&_journal_mode=WAL")
	if _, err := database.Exec(`CREATE TABLE items (value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO items VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	source, err := database.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if _, err := source.ExecContext(t.Context(), `BEGIN DEFERRED`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = source.ExecContext(context.Background(), `ROLLBACK`) })
	if err := source.QueryRowContext(t.Context(), `SELECT count(*) FROM items`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	releaseSnapshot, err := pinSQLiteWALSnapshot(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if releaseSnapshot != nil {
		t.Fatal("copy claimed ownership of the caller transaction")
	}
	if err := source.Raw(func(driverConn any) error {
		if driverConn.(*sqlite3.SQLiteConn).AutoCommit() {
			t.Fatal("copy ended the caller-owned transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ExecContext(t.Context(), `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
}

func TestPinSQLiteWALSnapshotSkipsOtherJournalModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	database := openBackupTestDB(t, path, "mode=rwc&_journal_mode=DELETE")
	source, err := database.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	releaseSnapshot, err := pinSQLiteWALSnapshot(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if releaseSnapshot != nil {
		t.Fatal("non-WAL source acquired a copy-owned transaction")
	}
	if err := source.Raw(func(driverConn any) error {
		if !driverConn.(*sqlite3.SQLiteConn).AutoCommit() {
			t.Fatal("non-WAL source left a transaction open")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func openBackupTestDB(t *testing.T, path, query string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", "file:"+EscapeSQLiteURIPath(filepath.ToSlash(path))+"?"+query)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := database.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return database
}
