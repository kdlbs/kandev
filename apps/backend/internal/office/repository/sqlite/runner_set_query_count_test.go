package sqlite_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// runnerSetQueryCounter counts how many times a query matching
// isRunnerSetQuery reaches the driver. It has no synchronization needs (no
// concurrency scenario, unlike commentWindowDriver) — just a counter plus
// the last matched query text, so a test can assert not only how many
// queries matched but that the surviving one is still the right shape.
type runnerSetQueryCounter struct {
	count    int32
	lastText string
}

func (c *runnerSetQueryCounter) inc(query string) {
	atomic.AddInt32(&c.count, 1)
	c.lastText = query
}
func (c *runnerSetQueryCounter) load() int32 { return atomic.LoadInt32(&c.count) }

// isRunnerSetQuery matches any query against the tasks table, not just the
// exact expected shape — a regression that adds a second, differently
// shaped query (e.g. a separate pre-count) must be caught too, not only a
// wholesale removal of the `COUNT(*) OVER()` clause.
func isRunnerSetQuery(query string) bool {
	return strings.Contains(strings.ToUpper(query), "FROM TASKS")
}

var runnerSetCounterGlobal struct {
	mu      sync.Mutex
	counter *runnerSetQueryCounter
}

func init() {
	sql.Register("sqlite3_runner_set_count_test", runnerSetCountDriver{})
}

type runnerSetCountDriver struct{}

func (runnerSetCountDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite3.SQLiteDriver{}).Open(name)
	if err != nil {
		return nil, err
	}
	runnerSetCounterGlobal.mu.Lock()
	counter := runnerSetCounterGlobal.counter
	runnerSetCounterGlobal.mu.Unlock()
	return &runnerSetCountConn{Conn: conn, counter: counter}, nil
}

type runnerSetCountConn struct {
	driver.Conn
	counter *runnerSetQueryCounter
}

func (c *runnerSetCountConn) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if c.counter != nil && isRunnerSetQuery(query) {
		c.counter.inc(query)
	}
	return queryer.QueryContext(ctx, query, args)
}

func (c *runnerSetCountConn) ExecContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, query, args)
}

func newCountingRunnerSetRepo(t *testing.T, counter *runnerSetQueryCounter) *sqlite.Repository {
	t.Helper()
	runnerSetCounterGlobal.mu.Lock()
	runnerSetCounterGlobal.counter = counter
	runnerSetCounterGlobal.mu.Unlock()
	t.Cleanup(func() {
		runnerSetCounterGlobal.mu.Lock()
		runnerSetCounterGlobal.counter = nil
		runnerSetCounterGlobal.mu.Unlock()
	})

	db, err := sqlx.Open("sqlite3_runner_set_count_test", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	createSearchTestSchema(t, repo)
	return repo
}

// TestListRunnerSetTaskIDs_IssuesExactlyOneQuery is the Review round 3
// regression test (R3REV-04). The round-2 atomicity fix replaced a
// count-then-list pair with a single `COUNT(*) OVER()` query so the runner
// set can't change between the two reads; nothing pinned that shape, so a
// regression back to two queries would ship silently.
func TestListRunnerSetTaskIDs_IssuesExactlyOneQuery(t *testing.T) {
	counter := &runnerSetQueryCounter{}
	repo := newCountingRunnerSetRepo(t, counter)
	ctx := context.Background()

	insertRunnerSetTask(t, repo, ctx, "task-a", "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	insertRunnerSetTask(t, repo, ctx, "task-b", "ws-1", "agent-1", "TODO", "2025-01-02 00:00:00")

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-1", "ws-1", 500)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 2 || len(ids) != 2 {
		t.Fatalf("ids/total = %v/%d, want 2 ids/2", ids, total)
	}
	if got := counter.load(); got != 1 {
		t.Fatalf("query count = %d, want 1 — ListRunnerSetTaskIDs must issue a single "+
			"query against tasks, not a count-then-list pair", got)
	}
	if !strings.Contains(strings.ToUpper(counter.lastText), "COUNT(*) OVER()") {
		t.Fatalf("surviving query = %q, want it to contain COUNT(*) OVER() — the single "+
			"query permitted must still be the atomic window-function shape", counter.lastText)
	}
}
