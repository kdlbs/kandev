package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// observerLogger returns a *logger.Logger backed by an observer core so
// log entries can be inspected in tests.
func observerLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

// memDB returns an in-memory SQLite connection for migration tests.
func memDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMigrateLogger_Apply_Success verifies that a fresh ALTER TABLE logs
// "migration applied" at INFO.
func TestMigrateLogger_Apply_Success(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	log, logs := observerLogger(t)
	m := NewMigrateLogger(db, log)
	m.Apply("t.col", `ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`)

	entries := logs.FilterMessage("migration applied").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 'migration applied' log, got %d", len(entries))
	}
	if entries[0].ContextMap()["name"] != "t.col" {
		t.Errorf("name field = %q, want %q", entries[0].ContextMap()["name"], "t.col")
	}
}

// TestMigrateLogger_Apply_Idempotent verifies that a duplicate-column ALTER
// produces no log output.
func TestMigrateLogger_Apply_Idempotent(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`); err != nil {
		t.Fatalf("initial alter: %v", err)
	}

	log, logs := observerLogger(t)
	m := NewMigrateLogger(db, log)
	// Same statement again - should be silent.
	m.Apply("t.col", `ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`)

	if logs.Len() != 0 {
		t.Errorf("expected no log entries for idempotent re-run, got %d: %v", logs.Len(), logs.All())
	}
}

// TestMigrateLogger_Apply_BrokenStatement verifies that a genuinely broken
// statement logs at WARN and not at INFO.
func TestMigrateLogger_Apply_BrokenStatement(t *testing.T) {
	db := memDB(t)

	log, logs := observerLogger(t)
	m := NewMigrateLogger(db, log)
	// Reference a table that does not exist - produces a real error.
	m.Apply("no_such.col", `ALTER TABLE no_such_table ADD COLUMN col TEXT`)

	warns := logs.FilterMessage("migration failed").All()
	if len(warns) != 1 {
		t.Fatalf("expected 1 'migration failed' WARN log, got %d", len(warns))
	}
	if warns[0].Level != zapcore.WarnLevel {
		t.Errorf("expected WARN level, got %v", warns[0].Level)
	}
}

func TestRequiredMigrateLogger_Apply_ReturnsUnexpectedFailure(t *testing.T) {
	db := memDB(t)
	m := NewRequiredMigrateLogger(db, nil)

	err := m.Apply("no_such.col", `ALTER TABLE no_such_table ADD COLUMN col TEXT`)
	if err == nil {
		t.Fatal("expected required migration failure")
	}
	if m.Err() == nil {
		t.Fatal("expected required migration logger to retain the failure")
	}
}

func TestRequiredMigrateLogger_Apply_AllowsIdempotentFailure(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`); err != nil {
		t.Fatalf("initial alter: %v", err)
	}

	m := NewRequiredMigrateLogger(db, nil)
	if err := m.Apply("t.col", `ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`); err != nil {
		t.Fatalf("idempotent migration: %v", err)
	}
	if m.Err() != nil {
		t.Fatalf("idempotent migration recorded an error: %v", m.Err())
	}
}

// TestMigrateLogger_Apply_NilLog verifies that a nil logger does not panic.
func TestMigrateLogger_Apply_NilLog(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	m := NewMigrateLogger(db, nil)
	// None of these should panic.
	m.Apply("t.col", `ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`)
	m.Apply("t.col", `ALTER TABLE t ADD COLUMN col TEXT DEFAULT ''`) // idempotent
	m.Apply("bad", `ALTER TABLE missing ADD COLUMN x TEXT`)
}

// TestRequiredMigrateLoggerContextCancelsActiveStatement proves cancellation
// reaches a migration after the driver has started executing it. The custom
// SQLite function gives the test a deterministic statement boundary instead
// of relying on a scheduler sleep or a large-data timing guess.
func TestRequiredMigrateLoggerContextCancelsActiveStatement(t *testing.T) {
	db := memDB(t)
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE t (value INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(value) VALUES (0)`); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve SQLite connection: %v", err)
	}
	if err := conn.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			t.Fatalf("driver connection = %T, want *sqlite3.SQLiteConn", driverConn)
		}
		return sqliteConn.RegisterFunc("wait_for_release", func() int64 {
			startedOnce.Do(func() { close(started) })
			<-release
			return 1
		}, true)
	}); err != nil {
		_ = conn.Close()
		t.Fatalf("register SQLite function: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("release SQLite connection: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewRequiredMigrateLoggerContext(db, nil, ctx).ApplyContext(
			ctx,
			"active.statement",
			`UPDATE t SET value = wait_for_release()`,
		)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("migration statement did not enter the SQLite function")
	}
	cancel()
	close(release)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active migration error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active migration did not stop after cancellation")
	}
}
