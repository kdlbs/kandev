package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func newTaskLockTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := OpenSQLite(filepath.Join(t.TempDir(), "tasklock.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, updated_at TIMESTAMP)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return sqlxDB
}

func TestLockTaskRowInTx_SQLiteNoOpSucceedsRegardlessOfExistence(t *testing.T) {
	sqlxDB := newTaskLockTestDB(t)
	ctx := context.Background()
	tx, err := sqlxDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// SQLite branch is a deliberate no-op: it must not fail even for a task
	// row that does not exist, because the guarantee it provides on SQLite
	// comes from the single-connection writer pool, not from this query.
	if err := LockTaskRowInTx(ctx, tx, "sqlite3", "does-not-exist"); err != nil {
		t.Fatalf("LockTaskRowInTx (sqlite no-op) = %v, want nil", err)
	}
}

func TestLockTaskRowInTx_SQLiteSerializesViaSingleWriterConnection(t *testing.T) {
	sqlxDB := newTaskLockTestDB(t)
	ctx := context.Background()
	if _, err := sqlxDB.Exec(`INSERT INTO tasks (id, updated_at) VALUES ('task-1', ?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	tx1, err := sqlxDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	if err := LockTaskRowInTx(ctx, tx1, "sqlite3", "task-1"); err != nil {
		t.Fatalf("LockTaskRowInTx tx1: %v", err)
	}

	// A second writer attempt against the same (MaxOpenConns=1) handle must
	// block until tx1 releases the single connection — proving the
	// serialization the design doc attributes to the writer pool rather
	// than to this function's SQLite branch.
	done := make(chan error, 1)
	go func() {
		_, execErr := sqlxDB.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = 'task-1'`, time.Now().UTC())
		done <- execErr
	}()

	select {
	case <-done:
		t.Fatal("second writer completed before the first transaction committed")
	case <-time.After(100 * time.Millisecond):
		// Expected: still blocked on the single connection.
	}

	if err := tx1.Commit(); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	select {
	case execErr := <-done:
		if execErr != nil {
			t.Fatalf("second writer error = %v, want nil", execErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second writer never completed after tx1 committed")
	}
}

func TestLockTaskRowInTx_PostgresBranchQueriesForUpdate(t *testing.T) {
	// The Postgres branch is exercised end-to-end by the environment-gated
	// Postgres suites in task/repository/sqlite (e.g.
	// TestTaskCleanupBarrierPostgres_*), which share this helper via
	// lockTaskRowInTx. This unit test only pins that a non-Postgres driver
	// name takes the no-op path and a Postgres driver name does not,
	// without requiring a live Postgres connection.
	sqlxDB := newTaskLockTestDB(t)
	ctx := context.Background()
	tx, err := sqlxDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Against a SQLite connection there is no "tasks" table shaped for a
	// pgx driver name to match query syntax, so asserting the pgx branch
	// is attempted (and fails, since this isn't really Postgres) is enough
	// to prove the dialect branch is live rather than silently skipped.
	err = LockTaskRowInTx(ctx, tx, "pgx", "task-1")
	if err == nil {
		t.Fatal("LockTaskRowInTx with pgx driver name against a non-Postgres connection = nil error, want a query error (FOR UPDATE is not valid SQLite syntax)")
	}
	if errors.Is(err, ErrTaskRowNotFound) {
		t.Fatalf("error = %v, want a syntax/execution error, not ErrTaskRowNotFound", err)
	}
}
