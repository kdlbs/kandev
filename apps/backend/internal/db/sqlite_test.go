package db_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type sqliteFactoryFixture struct{ writer, waiter, observer, reader *sqlx.DB }

func newSQLiteFactoryFixture(t *testing.T, ctx context.Context) sqliteFactoryFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "factory.db")
	open := func(read bool) *sqlx.DB {
		var raw *sql.DB
		var err error
		if read {
			raw, err = internaldb.OpenSQLiteReader(path)
		} else {
			raw, err = internaldb.OpenSQLite(path)
		}
		require.NoError(t, err)
		database := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { require.NoError(t, database.Close()) })
		require.NoError(t, database.PingContext(ctx))
		return database
	}
	f := sqliteFactoryFixture{writer: open(false)}
	_, err := f.writer.ExecContext(ctx, `CREATE TABLE admission_probe(id INTEGER PRIMARY KEY, revision INTEGER, title TEXT);
 INSERT INTO admission_probe VALUES(1,1,'retained');
 CREATE TABLE title_audit(id INTEGER);
 CREATE TRIGGER title_updated AFTER UPDATE OF title ON admission_probe BEGIN INSERT INTO title_audit VALUES(NEW.id); END`)
	require.NoError(t, err)
	f.waiter, f.observer, f.reader = open(false), open(false), open(true)
	var timeout int
	require.NoError(t, f.waiter.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout))
	require.Equal(t, 5000, timeout)
	_, err = f.observer.ExecContext(ctx, `PRAGMA busy_timeout=0`)
	require.NoError(t, err)
	return f
}

type sqliteFactoryResult struct {
	revision int
	err      error
}

func sqliteFactoryAttempt(ctx context.Context, writer *sqlx.DB, mode string) (result sqliteFactoryResult) {
	var tx *sqlx.Tx
	if mode == "Beginx" {
		tx, result.err = writer.Beginx()
	} else {
		tx, result.err = writer.BeginTxx(ctx, &sql.TxOptions{ReadOnly: mode == "ReadOnly"})
	}
	if result.err != nil {
		return result
	}
	defer func() {
		err := tx.Rollback()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			result.err = errors.Join(result.err, err)
		}
	}()
	if result.err = ctx.Err(); result.err != nil {
		return result
	}
	result.err = tx.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&result.revision)
	return result
}

// Correlate the parent inside driver BEGIN with its actual SQLite C worker.
// Repeat this observation across holder SQL and an independent BUSY probe.
func sqliteFactoryBeginWorker() string {
	buffer := make([]byte, 1<<20)
	sections := strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n")
	for _, parent := range sections {
		if !strings.Contains(parent, ".sqliteFactoryAttempt(") || !strings.Contains(parent, "(*SQLiteConn).begin(") {
			continue
		}
		id := strings.Fields(parent)[1]
		if strings.Contains(parent, "_Cfunc__sqlite3_step_row_internal(") {
			return id + "/" + id
		}
		for _, child := range sections {
			if strings.Contains(child, "_Cfunc__sqlite3_step_row_internal(") && strings.Contains(child, "created by github.com/mattn/go-sqlite3.(*SQLiteStmt).exec in goroutine "+id+"\n") {
				return id + "/" + strings.Fields(child)[1]
			}
		}
	}
	return ""
}

func awaitSQLiteFactoryBegin(t *testing.T, ctx context.Context, done <-chan sqliteFactoryResult) string {
	t.Helper()
	for {
		if worker := sqliteFactoryBeginWorker(); worker != "" {
			return worker
		}
		select {
		case result := <-done:
			t.Fatalf("writer returned before held SQLite BEGIN admission: revision=%d error=%v", result.revision, result.err)
		case <-ctx.Done():
			t.Fatal("driver BEGIN not observed", ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

func assertSQLiteFactoryBusy(t *testing.T, ctx context.Context, observer *sqlx.DB) {
	t.Helper()
	_, err := observer.ExecContext(ctx, `UPDATE admission_probe SET revision=revision WHERE 0`)
	var busy sqlite3.Error
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)
	t.Logf("held SQL writer probe: code=%d extended=%d", busy.Code, busy.ExtendedCode)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestSQLiteWriterTransactionAdmission(t *testing.T) {
	for _, mode := range []string{"BeginTxx", "ReadOnly", "Beginx"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			f := newSQLiteFactoryFixture(t, ctx)
			holder, err := f.writer.BeginTxx(ctx, nil)
			require.NoError(t, err)
			var workers sync.WaitGroup
			t.Cleanup(func() { cancel(); _ = holder.Rollback(); workers.Wait() })
			_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=2 WHERE id=1`)
			require.NoError(t, err)
			assertSQLiteFactoryBusy(t, ctx, f.observer)
			done := make(chan sqliteFactoryResult, 1)
			workers.Add(1)
			go func() { defer workers.Done(); done <- sqliteFactoryAttempt(ctx, f.waiter, mode) }()
			worker := awaitSQLiteFactoryBegin(t, ctx, done)
			_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=3 WHERE id=1`)
			require.NoError(t, err)
			assertSQLiteFactoryBusy(t, ctx, f.observer)
			require.Equal(t, worker, sqliteFactoryBeginWorker(), "same genuine BEGIN C worker across holder mutation/probe")
			select {
			case result := <-done:
				t.Fatalf("waiter returned with held writer: %+v", result)
			default:
			}
			t.Logf("%s physical BEGIN parent/worker=%s before/after SQL proof", mode, worker)
			require.NoError(t, holder.Commit())
			select {
			case result := <-done:
				require.NoError(t, result.err)
				require.Equal(t, 3, result.revision)
			case <-ctx.Done():
				t.Fatal("waiter did not settle", ctx.Err())
			}
			workers.Wait()
			require.NoError(t, f.waiter.PingContext(ctx))
			require.Zero(t, f.waiter.Stats().InUse)
			var audited int
			require.NoError(t, f.reader.QueryRowContext(ctx, `SELECT count(*) FROM title_audit`).Scan(&audited))
			require.Zero(t, audited, "BEGIN must not touch retained title or trigger")
		})
	}
}

func takeSQLiteFactoryResult(t *testing.T, ctx context.Context, done <-chan sqliteFactoryResult) sqliteFactoryResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		t.Fatal("factory waiter did not settle", ctx.Err())
		return sqliteFactoryResult{err: ctx.Err()}
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestSQLiteWriterTransactionCancellation(t *testing.T) {
	for _, mode := range []string{"already_cancelled", "cancel_while_held", "busy_expiry"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			f := newSQLiteFactoryFixture(t, ctx)
			holder, err := f.writer.BeginTxx(ctx, nil)
			require.NoError(t, err)
			waiterCtx, cancelWaiter := context.WithCancel(ctx)
			var workers sync.WaitGroup
			t.Cleanup(func() { cancelWaiter(); cancel(); _ = holder.Rollback(); workers.Wait() })
			_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=2 WHERE id=1`)
			require.NoError(t, err)
			if mode == "already_cancelled" {
				cancelWaiter()
			}
			done := make(chan sqliteFactoryResult, 1)
			started := time.Now()
			workers.Add(1)
			go func() { defer workers.Done(); done <- sqliteFactoryAttempt(waiterCtx, f.waiter, "BeginTxx") }()
			if mode != "already_cancelled" {
				worker := awaitSQLiteFactoryBegin(t, ctx, done)
				_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=3 WHERE id=1`)
				require.NoError(t, err)
				assertSQLiteFactoryBusy(t, ctx, f.observer)
				require.Equal(t, worker, sqliteFactoryBeginWorker())
				if mode == "cancel_while_held" {
					cancelWaiter()
				}
			}
			result := takeSQLiteFactoryResult(t, ctx, done)
			workers.Wait()
			require.Error(t, result.err)
			require.Zero(t, result.revision, "failed entry never reads or admits authority")
			var native sqlite3.Error
			if mode == "busy_expiry" {
				require.ErrorAs(t, result.err, &native)
				require.Equal(t, sqlite3.ErrBusy, native.Code)
			} else {
				require.ErrorIs(t, waiterCtx.Err(), context.Canceled)
				require.True(t, errors.Is(result.err, context.Canceled) || errors.As(result.err, &native), "real driver cancellation or native failure required")
			}
			t.Logf("%s settled with holder STILL locked after %s: %v, code=%d extended=%d", mode, time.Since(started), result.err, native.Code, native.ExtendedCode)
			assertSQLiteFactoryBusy(t, ctx, f.observer)
			require.NoError(t, f.waiter.PingContext(ctx))
			require.Zero(t, f.waiter.Stats().InUse)
			var current int
			require.NoError(t, f.reader.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&current))
			require.Equal(t, 1, current)
			require.NoError(t, holder.Commit())
			reused := sqliteFactoryAttempt(ctx, f.waiter, "BeginTxx")
			require.NoError(t, reused.err)
			if mode == "already_cancelled" {
				require.Equal(t, 2, reused.revision)
			} else {
				require.Equal(t, 3, reused.revision)
			}
			require.Zero(t, f.waiter.Stats().InUse)
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestSQLiteWriterTransactionRollbackReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f := newSQLiteFactoryFixture(t, ctx)
	for _, mode := range []string{"sql_error", "rollback", "cancel_returned_tx"} {
		t.Run(mode, func(t *testing.T) {
			txCtx, cancelTx := context.WithCancel(ctx)
			defer cancelTx()
			tx, err := f.waiter.BeginTxx(txCtx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			_, err = tx.ExecContext(txCtx, `UPDATE admission_probe SET revision=9 WHERE id=1`)
			require.NoError(t, err)
			if mode == "sql_error" {
				_, err = tx.ExecContext(txCtx, `INSERT INTO admission_probe VALUES(1,9,'duplicate')`)
				require.Error(t, err)
			}
			if mode == "cancel_returned_tx" {
				cancelTx()
			}
			err = tx.Rollback()
			require.True(t, err == nil || errors.Is(err, sql.ErrTxDone))
			require.NoError(t, f.waiter.PingContext(ctx), "checkout joins cancellation rollback")
			require.Zero(t, f.waiter.Stats().InUse)
			result := sqliteFactoryAttempt(ctx, f.waiter, "BeginTxx")
			require.NoError(t, result.err)
			require.Equal(t, 1, result.revision)
		})
	}
	tx, err := f.waiter.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `UPDATE admission_probe SET revision=4 WHERE id=1`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	result := sqliteFactoryAttempt(ctx, f.waiter, "BeginTxx")
	require.NoError(t, result.err)
	require.Equal(t, 4, result.revision)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestSQLiteReaderProgressDuringWriterTransaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	f := newSQLiteFactoryFixture(t, ctx)
	holder, err := f.writer.BeginTxx(ctx, nil)
	require.NoError(t, err)
	var workers sync.WaitGroup
	t.Cleanup(func() { cancel(); _ = holder.Rollback(); workers.Wait() })
	_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=2 WHERE id=1`)
	require.NoError(t, err)
	done := make(chan sqliteFactoryResult, 1)
	workers.Add(1)
	go func() { defer workers.Done(); done <- sqliteFactoryAttempt(ctx, f.waiter, "BeginTxx") }()
	worker := awaitSQLiteFactoryBegin(t, ctx, done)
	reader, err := f.reader.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Rollback() })
	var revision int
	require.NoError(t, reader.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&revision))
	require.Equal(t, 1, revision)
	// A second physical reader progresses without waiting for either transaction.
	require.NoError(t, f.reader.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&revision))
	require.Equal(t, 1, revision)
	_, err = holder.ExecContext(ctx, `UPDATE admission_probe SET revision=3 WHERE id=1`)
	require.NoError(t, err)
	assertSQLiteFactoryBusy(t, ctx, f.observer)
	require.Equal(t, worker, sqliteFactoryBeginWorker())
	require.NoError(t, holder.Commit())
	result := takeSQLiteFactoryResult(t, ctx, done)
	workers.Wait()
	require.NoError(t, result.err)
	require.Equal(t, 3, result.revision)
	require.NoError(t, reader.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&revision))
	require.Equal(t, 1, revision, "reader retains its established snapshot across writer commit")
	_, err = reader.ExecContext(ctx, `UPDATE admission_probe SET title='forbidden' WHERE id=1`)
	require.Error(t, err)
	require.NoError(t, reader.Rollback())
	require.NoError(t, f.reader.QueryRowContext(ctx, `SELECT revision FROM admission_probe WHERE id=1`).Scan(&revision))
	require.Equal(t, 3, revision)
	require.Zero(t, f.reader.Stats().InUse)
}
