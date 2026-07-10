package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/jobs"
)

const fakePostgresStatsDriverName = "kandev-system-database-stats-postgres"

var registerFakePostgresStatsDriverOnce sync.Once

type fakePostgresStatsDriver struct{}

func (fakePostgresStatsDriver) Open(string) (driver.Conn, error) {
	return fakePostgresStatsConn{}, nil
}

type fakePostgresStatsConn struct{}

func (fakePostgresStatsConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare is not implemented")
}

func (fakePostgresStatsConn) Close() error {
	return nil
}

func (fakePostgresStatsConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions are not implemented")
}

func (fakePostgresStatsConn) QueryContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	normalized := strings.Join(strings.Fields(query), " ")
	switch normalized {
	case "SELECT pg_database_size(current_database())":
		return newFakeRows([]string{"pg_database_size"}, []driver.Value{int64(4096)}), nil
	case "SELECT value FROM kandev_meta WHERE key = $1":
		if len(args) != 1 || args[0].Value != "kandev_version" {
			return nil, fmt.Errorf("unexpected args for schema version: %#v", args)
		}
		return newFakeRows([]string{"value"}, []driver.Value{"v0.99.0"}), nil
	default:
		if strings.HasPrefix(normalized, "PRAGMA ") {
			return nil, fmt.Errorf(`ERROR: syntax error at or near "PRAGMA" (SQLSTATE 42601)`)
		}
		return nil, fmt.Errorf("unexpected query: %s", normalized)
	}
}

type fakeRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

func newFakeRows(columns []string, values []driver.Value) *fakeRows {
	return &fakeRows{columns: columns, values: values}
}

func (r *fakeRows) Columns() []string {
	return r.columns
}

func (r *fakeRows) Close() error {
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	copy(dest, r.values)
	return nil
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// stubBus is a minimal in-memory EventBus that records published events.
type stubBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (s *stubBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}
func (s *stubBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) { return nil, nil }
func (s *stubBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (s *stubBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (s *stubBus) Close()            {}
func (s *stubBus) IsConnected() bool { return true }

var _ bus.EventBus = (*stubBus)(nil)
var _ = events.SystemJobUpdate

// newTestPool opens a temp SQLite at <dataDir>/kandev.db and seeds it with a
// kandev_meta row plus a couple of user tables and rows so VACUUM has
// something to reclaim.
func newTestPool(t *testing.T, dataDir string) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(dataDir, "kandev.db")
	writerRaw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	readerRaw, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteReader: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	pool := db.NewPool(writer, reader)

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`INSERT OR REPLACE INTO kandev_meta (key, value) VALUES ('kandev_version', 'v0.99.0')`,
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT, payload BLOB)`,
		`CREATE TABLE IF NOT EXISTS sessions_t (id INTEGER PRIMARY KEY, data TEXT)`,
	}
	for _, s := range stmts {
		if _, err := writer.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	// Insert ~100KB of churn to give VACUUM something to reclaim.
	for i := 0; i < 200; i++ {
		blob := make([]byte, 1024)
		for j := range blob {
			blob[j] = byte(j % 256)
		}
		if _, err := writer.Exec(`INSERT INTO users (name, payload) VALUES (?, ?)`, "row", blob); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if _, err := writer.Exec(`DELETE FROM users WHERE id % 2 = 0`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	return pool, dbPath
}

// newTestService wires a Service with a Tracker over the stubBus.
func newTestService(t *testing.T) (*Service, *jobs.Tracker, *stubBus, string) {
	t.Helper()
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	pool, _ := newTestPool(t, dataDir)
	t.Cleanup(func() { _ = pool.Close() })

	stub := &stubBus{}
	log := newTestLogger(t)
	tracker := jobs.NewTracker(stub, log)
	dirs := ResetDirs{
		Worktrees: filepath.Join(tmp, "worktrees"),
		Repos:     filepath.Join(tmp, "repos"),
		Sessions:  filepath.Join(tmp, "sessions"),
		Tasks:     filepath.Join(tmp, "tasks"),
		QuickChat: filepath.Join(tmp, "quick-chat"),
	}
	for _, d := range []string{dirs.Worktrees, dirs.Repos, dirs.Sessions, dirs.Tasks, dirs.QuickChat} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
		// Write a sentinel file so we can prove RemoveAll wiped it.
		if err := os.WriteFile(filepath.Join(d, "sentinel"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write sentinel: %v", err)
		}
	}
	svc := NewService(pool, dataDir, dirs, tracker, log)
	return svc, tracker, stub, dataDir
}

func newFakePostgresStatsPool(t *testing.T) *db.Pool {
	t.Helper()
	registerFakePostgresStatsDriverOnce.Do(func() {
		sql.Register(fakePostgresStatsDriverName, fakePostgresStatsDriver{})
	})
	raw, err := sql.Open(fakePostgresStatsDriverName, "")
	if err != nil {
		t.Fatalf("open fake postgres: %v", err)
	}
	pg := sqlx.NewDb(raw, "pgx")
	pool := db.NewPool(pg, pg)
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// waitForState mirrors jobs.waitForState — wait until the tracker reports
// the target state for the given id, or fail after 2s.
func waitForState(t *testing.T, tracker *jobs.Tracker, id string, target jobs.State) *jobs.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j := tracker.Get(id)
		if j != nil && j.State == target {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach state %s within 2s; last = %+v", id, target, tracker.Get(id))
	return nil
}

func TestStats_ReturnsPathSizeAndSchemaVersion(t *testing.T) {
	svc, _, _, dataDir := newTestService(t)

	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	wantPath := filepath.Join(dataDir, "kandev.db")
	if stats.Path != wantPath {
		t.Errorf("Path = %q, want %q", stats.Path, wantPath)
	}
	if stats.SizeBytes <= 0 {
		t.Errorf("SizeBytes = %d, want > 0", stats.SizeBytes)
	}
	if stats.SchemaVersion != "v0.99.0" {
		t.Errorf("SchemaVersion = %q, want v0.99.0", stats.SchemaVersion)
	}
	if stats.LastBackupAt != nil {
		t.Errorf("LastBackupAt = %v, want nil (no backups yet)", *stats.LastBackupAt)
	}
}

func TestStats_PostgresDoesNotUseSQLitePragmas(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewService(newFakePostgresStatsPool(t), dataDir, ResetDirs{}, nil, nil)

	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Driver != "postgres" {
		t.Errorf("Driver = %q, want postgres", stats.Driver)
	}
	if stats.Path != "" {
		t.Errorf("Path = %q, want empty for postgres", stats.Path)
	}
	if stats.SizeBytes != 4096 {
		t.Errorf("SizeBytes = %d, want 4096", stats.SizeBytes)
	}
	if stats.WALSizeBytes != 0 {
		t.Errorf("WALSizeBytes = %d, want 0 for postgres", stats.WALSizeBytes)
	}
	if stats.SchemaVersion != "v0.99.0" {
		t.Errorf("SchemaVersion = %q, want v0.99.0", stats.SchemaVersion)
	}
	if stats.LastBackupAt != nil {
		t.Errorf("LastBackupAt = %v, want nil for postgres", *stats.LastBackupAt)
	}
}

func TestStats_LastBackupAtPicksNewestFile(t *testing.T) {
	svc, _, _, dataDir := newTestService(t)

	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	older := filepath.Join(backupDir, "kandev-old.db")
	newer := filepath.Join(backupDir, "kandev-new.db")
	if err := os.WriteFile(older, []byte("a"), 0o644); err != nil {
		t.Fatalf("write older: %v", err)
	}
	earlier := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, earlier, earlier); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.WriteFile(newer, []byte("b"), 0o644); err != nil {
		t.Fatalf("write newer: %v", err)
	}

	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.LastBackupAt == nil {
		t.Fatalf("LastBackupAt should be set when a backup file exists")
	}
	if !stats.LastBackupAt.After(earlier.Add(time.Hour)) {
		t.Errorf("LastBackupAt %v should be more recent than %v", *stats.LastBackupAt, earlier)
	}
}

func TestHandleStats_Returns200JSON(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/db", HandleStats(svc))

	req := httpGet(t, "/db")
	w := serveHTTP(r, req)
	if w.Code != 200 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, `"driver"`) || !contains(body, `"path"`) || !contains(body, `"schema_version"`) {
		t.Errorf("body missing fields: %s", body)
	}
}
