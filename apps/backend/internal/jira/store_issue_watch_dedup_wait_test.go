package jira

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	sqlite3 "github.com/mattn/go-sqlite3"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

type sqliteBeginBarrier struct {
	blocked        chan struct{}
	enabled        atomic.Bool
	onBeginSuccess func()
}

func (b *sqliteBeginBarrier) signalBlocked() {
	if !b.enabled.Load() {
		return
	}
	select {
	case b.blocked <- struct{}{}:
	default:
	}
}

// instrumentedSQLiteDriver wraps the real SQLite driver so the test can
// observe the claimant receiving SQLITE_BUSY for BEGIN IMMEDIATE. The holder
// keeps the production writer transaction open, then installs the replacement
// mapping before the claimant retries.
type instrumentedSQLiteDriver struct {
	base    *sqlite3.SQLiteDriver
	barrier *sqliteBeginBarrier
}

func (d *instrumentedSQLiteDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &instrumentedSQLiteConn{Conn: conn, barrier: d.barrier}, nil
}

type instrumentedSQLiteConn struct {
	driver.Conn
	barrier *sqliteBeginBarrier
}

func (c *instrumentedSQLiteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	for {
		tx, err := c.begin(ctx, opts)
		if err == nil {
			if c.barrier.onBeginSuccess != nil {
				c.barrier.onBeginSuccess()
			}
			return tx, nil
		}
		if !isSQLiteBusy(err) {
			return nil, err
		}
		c.barrier.signalBlocked()
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *instrumentedSQLiteConn) begin(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if begin, ok := c.Conn.(driver.ConnBeginTx); ok {
		return begin.BeginTx(ctx, opts)
	}
	// The SQLite driver only implements the deprecated driver.Conn.Begin.
	return c.Conn.Begin() //nolint:staticcheck
}

func isSQLiteBusy(err error) bool {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked
}

func (c *instrumentedSQLiteConn) ExecContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, query, args)
}

func (c *instrumentedSQLiteConn) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, query, args)
}

func (c *instrumentedSQLiteConn) PrepareContext(
	ctx context.Context, query string,
) (driver.Stmt, error) {
	preparer, ok := c.Conn.(driver.ConnPrepareContext)
	if !ok {
		return c.Prepare(query)
	}
	return preparer.PrepareContext(ctx, query)
}

var instrumentedSQLiteDriverSeq atomic.Uint64

func registerInstrumentedSQLiteDriver(t *testing.T, barrier *sqliteBeginBarrier) string {
	t.Helper()
	name := fmt.Sprintf("jira-test-sqlite-%d", instrumentedSQLiteDriverSeq.Add(1))
	sql.Register(name, &instrumentedSQLiteDriver{base: &sqlite3.SQLiteDriver{}, barrier: barrier})
	return name
}

func instrumentedSQLiteDSN(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return fmt.Sprintf(
		"file:%s?_foreign_keys=on&_mode=rwc&_busy_timeout=0&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared&_txlock=immediate",
		absolute,
	)
}

type issueWatchClaimResult struct {
	reserved bool
	err      error
}

func TestStore_IssueWatchDedup_WaitingClaimantPreservesReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	path := filepath.Join(t.TempDir(), "jira-waiting-claimant.db")
	holderRaw, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open holder: %v", err)
	}
	holderDB := sqlx.NewDb(holderRaw, "sqlite3")
	t.Cleanup(func() { _ = holderDB.Close() })
	holderStore, err := NewStore(holderDB, holderDB)
	if err != nil {
		t.Fatalf("holder store: %v", err)
	}
	if _, err := holderDB.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		archived_at DATETIME,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks fixture: %v", err)
	}
	watch := createDedupWatch(t, holderStore, "ws-1")
	seedDedupMapping(t, holderStore, watch.ID, "PROJ-1", "task-missing", nil)
	before := readIssueWatchTaskRecord(t, holderStore, watch.ID, "PROJ-1")

	barrier := &sqliteBeginBarrier{blocked: make(chan struct{}, 1)}
	driverName := registerInstrumentedSQLiteDriver(t, barrier)
	claimantRaw, err := sql.Open(driverName, instrumentedSQLiteDSN(path))
	if err != nil {
		t.Fatalf("open claimant: %v", err)
	}
	claimantRaw.SetMaxOpenConns(1)
	claimantRaw.SetMaxIdleConns(1)
	claimantDB := sqlx.NewDb(claimantRaw, "sqlite3")
	t.Cleanup(func() { _ = claimantDB.Close() })
	claimantStore, err := NewStore(claimantDB, claimantDB)
	if err != nil {
		t.Fatalf("claimant store: %v", err)
	}

	tx, err := holderDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE jira_issue_watches SET updated_at = updated_at WHERE id = ?`), watch.ID); err != nil {
		t.Fatalf("acquire writer: %v", err)
	}

	barrier.enabled.Store(true)
	var wg sync.WaitGroup
	results := make(chan issueWatchClaimResult, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		reserved, err := claimantStore.ReserveIssueWatchTask(
			ctx, watch.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
		results <- issueWatchClaimResult{reserved: reserved, err: err}
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

	select {
	case <-barrier.blocked:
	case <-ctx.Done():
		t.Fatal("claimant did not receive SQLITE_BUSY for BEGIN IMMEDIATE")
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO tasks (id) VALUES (?)`), "task-replacement"); err != nil {
		t.Fatalf("create replacement task: %v", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE jira_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND issue_key = ?`),
		"task-replacement", watch.ID, "PROJ-1"); err != nil {
		t.Fatalf("attach replacement task: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit holder: %v", err)
	}

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("waiting claim: %v", result.err)
		}
		if result.reserved {
			t.Fatal("waiting claimant must lose to the attached replacement task")
		}
	case <-ctx.Done():
		t.Fatal("waiting claimant did not finish")
	}
	after := readIssueWatchTaskRecord(t, holderStore, watch.ID, "PROJ-1")
	if after.ID != before.ID || after.TaskID != "task-replacement" {
		t.Fatalf("replacement mapping changed: before=%+v after=%+v", before, after)
	}
}

func TestStore_IssueWatchDedup_CancelledClaimAfterBegin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path := filepath.Join(t.TempDir(), "jira-cancelled-claim.db")
	baseRaw, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open base: %v", err)
	}
	baseDB := sqlx.NewDb(baseRaw, "sqlite3")
	t.Cleanup(func() { _ = baseDB.Close() })
	baseStore, err := NewStore(baseDB, baseDB)
	if err != nil {
		t.Fatalf("base store: %v", err)
	}
	if _, err := baseDB.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		archived_at DATETIME,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks fixture: %v", err)
	}
	watch := createDedupWatch(t, baseStore, "ws-1")
	seedDedupMapping(t, baseStore, watch.ID, "PROJ-1", "task-missing", nil)
	before := readIssueWatchTaskRecord(t, baseStore, watch.ID, "PROJ-1")

	barrier := &sqliteBeginBarrier{blocked: make(chan struct{}, 1), onBeginSuccess: cancel}
	driverName := registerInstrumentedSQLiteDriver(t, barrier)
	claimantRaw, err := sql.Open(driverName, instrumentedSQLiteDSN(path))
	if err != nil {
		t.Fatalf("open claimant: %v", err)
	}
	claimantRaw.SetMaxOpenConns(1)
	claimantRaw.SetMaxIdleConns(1)
	claimantDB := sqlx.NewDb(claimantRaw, "sqlite3")
	t.Cleanup(func() { _ = claimantDB.Close() })
	claimantStore, err := NewStore(claimantDB, claimantDB)
	if err != nil {
		t.Fatalf("claimant store: %v", err)
	}
	barrier.enabled.Store(true)

	reserved, err := claimantStore.ReserveIssueWatchTask(
		ctx, watch.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled claim error = %v, want context.Canceled", err)
	}
	if reserved {
		t.Fatal("cancelled claim must not reserve")
	}
	after := readIssueWatchTaskRecord(t, baseStore, watch.ID, "PROJ-1")
	if after.ID != before.ID || after.TaskID != before.TaskID {
		t.Fatalf("cancelled claim changed mapping: before=%+v after=%+v", before, after)
	}
}

func TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(6)
	database.SetMaxIdleConns(6)
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		archived_at TIMESTAMP,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks fixture: %v", err)
	}
	watch := createDedupWatch(t, store, "ws-1")
	seedDedupMapping(t, store, watch.ID, "PROJ-1", "task-missing", nil)
	before := readIssueWatchTaskRecord(t, store, watch.ID, "PROJ-1")

	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockJiraIssueWatchClaim(ctx, tx, "pgx", watch.ID, "PROJ-1"); err != nil {
		t.Fatalf("hold claim lock: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan issueWatchClaimResult, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		reserved, err := store.ReserveIssueWatchTask(
			ctx, watch.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
		results <- issueWatchClaimResult{reserved: reserved, err: err}
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

	waitForWaitingJiraClaimLock(t, ctx, database, watch.ID, "PROJ-1")
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks (id) VALUES ($1)`, "task-replacement"); err != nil {
		t.Fatalf("create replacement task: %v", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE jira_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND issue_key = ?`),
		"task-replacement", watch.ID, "PROJ-1"); err != nil {
		t.Fatalf("attach replacement task: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit holder: %v", err)
	}

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("waiting claim: %v", result.err)
		}
		if result.reserved {
			t.Fatal("waiting claimant must lose to the attached replacement task")
		}
	case <-ctx.Done():
		t.Fatal("waiting claimant did not finish")
	}
	after := readIssueWatchTaskRecord(t, store, watch.ID, "PROJ-1")
	if after.ID != before.ID || after.TaskID != "task-replacement" {
		t.Fatalf("replacement mapping changed: before=%+v after=%+v", before, after)
	}
}

func waitForWaitingJiraClaimLock(
	t *testing.T, ctx context.Context, database *sqlx.DB, watchID, issueKey string,
) {
	t.Helper()
	key := jiraIssueWatchClaimLockKey(watchID, issueKey)
	for {
		var waiting bool
		if err := database.GetContext(ctx, &waiting, `
			SELECT EXISTS (
				SELECT 1 FROM pg_locks
				WHERE locktype = 'advisory' AND NOT granted
				  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
				  AND objsubid = 1
				  AND (classid::bigint::bit(32) || objid::bigint::bit(32)) =
				      hashtextextended($1, 0)::bit(64)
			)`, key); err != nil {
			t.Fatalf("read advisory waiters: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("claimant did not wait on the advisory lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
