package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

func TestClarificationReadsPreserveInteractiveCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clarification-admission.db")
	writerRaw, err := internaldb.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open SQLite writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")

	barrier := newClarificationQueryBarrier()
	readerRaw := sql.OpenDB(&clarificationBarrierConnector{path: path, barrier: barrier})
	readerRaw.SetMaxOpenConns(4)
	readerRaw.SetMaxIdleConns(4)
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	pool := internaldb.NewPool(writer, reader)
	t.Cleanup(func() {
		barrier.unblock()
		_ = pool.Close()
	})

	repo, err := NewWithDBContext(context.Background(), writer, reader, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "task-read-fixture", OwnerPackage: "task", RequiredTables: []string{"tasks"},
		Sweep: startup.StepStoresRepositories,
	}})
	if err != nil {
		t.Fatalf("create required-store tracker: %v", err)
	}
	if err := tracker.RecordSuccess("task-read-fixture"); err != nil {
		t.Fatalf("record task store readiness: %v", err)
	}
	health := requiredstores.NewHealth(tracker, pool, nil)
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial health check: %v", err)
	}

	barrier.arm()
	firstResult := make(chan error, 1)
	go func() {
		_, err := repo.ListUnresolvedClarificationBundles(context.Background(), models.ListClarificationBundlesOptions{
			Unscoped: true, Limit: 1,
		})
		firstResult <- err
	}()
	awaitSignal(t, barrier.started, "first clarification query")

	const queuedReads = 3
	observed := make(chan struct{}, queuedReads)
	results := make(chan error, queuedReads)
	cancels := make([]context.CancelFunc, 0, queuedReads)
	var workers sync.WaitGroup
	for i := 0; i < queuedReads; i++ {
		parent, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		ctx := &clarificationObservedContext{Context: parent, observed: observed}
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			if index%2 == 0 {
				_, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
					Unscoped: true, Limit: 1,
				})
				results <- err
				return
			}
			_, err := repo.CountHiddenClarificationBundles(ctx, models.ListClarificationBundlesOptions{
				Unscoped: true, Limit: 1,
			})
			results <- err
		}(i)
	}
	for i := 0; i < queuedReads; i++ {
		awaitSignal(t, observed, "queued clarification read admission attempt")
	}

	if got := reader.Stats().InUse; got != 1 {
		t.Fatalf("reader connections in use while one clarification query is held = %d, want 1", got)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, taskReadErr := repo.GetTask(readCtx, "missing-task")
	readCancel()
	if !errors.Is(taskReadErr, ErrTaskNotFound) {
		t.Errorf("ordinary task read error = %v, want ErrTaskNotFound after a successful query", taskReadErr)
	}

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
	healthErr := health.Check(healthCtx)
	healthCancel()
	if healthErr != nil {
		t.Errorf("required-store health check failed while clarification work was held: %v", healthErr)
	}

	for _, cancel := range cancels {
		cancel()
	}
	for i := 0; i < queuedReads; i++ {
		if err := awaitError(t, results, "canceled queued clarification read"); !errors.Is(err, context.Canceled) {
			t.Errorf("queued clarification read error = %v, want context.Canceled", err)
		}
	}
	barrier.unblock()
	if err := awaitError(t, firstResult, "held clarification query"); err != nil {
		t.Errorf("held clarification query failed after release: %v", err)
	}
	workers.Wait()
}

func TestClarificationReadAdmissionExpiresWhileQueued(t *testing.T) {
	repo := newRepoForSessionTests(t)
	_, release, _, err := repo.beginClarificationRead(context.Background())
	if err != nil {
		t.Fatalf("admit first clarification read: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, _, _, err := repo.beginClarificationRead(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued clarification read error = %v, want context deadline exceeded", err)
	}

	release()
	if _, nextRelease, _, err := repo.beginClarificationRead(context.Background()); err != nil {
		t.Fatalf("admission remained occupied after queued expiry: %v", err)
	} else {
		nextRelease()
	}
}

func TestClarificationReadAdmissionReleasesAfterSQLError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if err := repo.db.Close(); err != nil {
		t.Fatalf("close SQLite before failed read: %v", err)
	}
	if _, err := repo.CountHiddenClarificationBundles(
		context.Background(), models.ListClarificationBundlesOptions{Unscoped: true},
	); err == nil {
		t.Fatal("count on a closed database succeeded")
	}
	if _, release, _, err := repo.beginClarificationRead(context.Background()); err != nil {
		t.Fatalf("admission remained occupied after SQL error: %v", err)
	} else {
		release()
	}
}

type clarificationObservedContext struct {
	context.Context
	observed chan<- struct{}
	once     sync.Once
}

func (c *clarificationObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { c.observed <- struct{}{} })
	return c.Context.Done()
}

type clarificationQueryBarrier struct {
	mu      sync.Mutex
	armed   bool
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newClarificationQueryBarrier() *clarificationQueryBarrier {
	return &clarificationQueryBarrier{started: make(chan struct{}, 16), release: make(chan struct{})}
}

func (b *clarificationQueryBarrier) arm() {
	b.mu.Lock()
	b.armed = true
	b.mu.Unlock()
}

func (b *clarificationQueryBarrier) unblock() {
	b.once.Do(func() { close(b.release) })
}

func (b *clarificationQueryBarrier) wait(ctx context.Context, query string) error {
	if !strings.Contains(query, "FROM task_session_messages m") {
		return nil
	}
	b.mu.Lock()
	armed := b.armed
	b.mu.Unlock()
	if !armed {
		return nil
	}
	select {
	case b.started <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type clarificationBarrierConnector struct {
	path    string
	barrier *clarificationQueryBarrier
}

func (c *clarificationBarrierConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_foreign_keys=on&_busy_timeout=5000", c.path)
	conn, err := (&sqlite3.SQLiteDriver{}).Open(dsn)
	if err != nil {
		return nil, err
	}
	return &clarificationBarrierConn{Conn: conn, barrier: c.barrier}, nil
}

func (c *clarificationBarrierConnector) Driver() driver.Driver {
	return clarificationBarrierDriver{connector: c}
}

type clarificationBarrierDriver struct {
	connector *clarificationBarrierConnector
}

func (d clarificationBarrierDriver) Open(string) (driver.Conn, error) {
	return d.connector.Connect(context.Background())
}

type clarificationBarrierConn struct {
	driver.Conn
	barrier *clarificationQueryBarrier
}

func (c *clarificationBarrierConn) QueryContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	if err := c.barrier.wait(ctx, query); err != nil {
		return nil, err
	}
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, query, args)
}

func (c *clarificationBarrierConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func awaitError(t *testing.T, ch <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}
