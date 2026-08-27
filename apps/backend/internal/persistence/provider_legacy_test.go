package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func seedLegacyTaskDatabase(t *testing.T, path string) []byte {
	return seedTaskDatabase(t, path, "task-1", "legacy task")
}

func seedTaskDatabase(t *testing.T, path, id, title string) []byte {
	t.Helper()
	database, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT NOT NULL)`); err != nil {
		_ = database.Close()
		t.Fatalf("create tasks: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO tasks (id, title) VALUES (?, ?)`, id, title); err != nil {
		_ = database.Close()
		t.Fatalf("insert task: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy database: %v", err)
	}
	return contents
}

func TestProvideAdoptsLegacySQLiteWithoutDeletingSource(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	newPath := filepath.Join(homeDir, "data", "kandev.db")
	legacyContents := seedLegacyTaskDatabase(t, legacyPath)

	cfg := &config.Config{HomeDir: homeDir}
	pool, cleanup, err := Provide(cfg, nil, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	var title string
	if err := pool.Reader().Get(&title, `SELECT title FROM tasks WHERE id = 'task-1'`); err != nil {
		t.Fatalf("read adopted task: %v", err)
	}
	if title != "legacy task" {
		t.Fatalf("adopted task title = %q, want %q", title, "legacy task")
	}

	gotLegacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read retained legacy database: %v", err)
	}
	if string(gotLegacy) != string(legacyContents) {
		t.Fatalf("legacy database changed during adoption")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("adopted database missing at %s: %v", newPath, err)
	}
}

func TestProvideAdoptsCommittedWALDataAndRetainsLegacySidecars(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	newPath := filepath.Join(homeDir, "data", "kandev.db")
	openWALLegacyTaskDatabase(t, legacyPath)

	legacyContents := readFileForTest(t, legacyPath)
	walContents := readFileForTest(t, legacyPath+"-wal")
	if _, err := os.Stat(legacyPath + "-shm"); err != nil {
		t.Fatalf("WAL legacy SHM sidecar missing before adoption: %v", err)
	}
	cfg := &config.Config{HomeDir: homeDir}
	pool, cleanup, err := Provide(cfg, nil, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	var title string
	if err := pool.Reader().Get(&title, `SELECT title FROM tasks WHERE id = 'task-wal'`); err != nil {
		t.Fatalf("read adopted WAL task: %v", err)
	}
	if title != "wal task" {
		t.Fatalf("adopted WAL task title = %q, want %q", title, "wal task")
	}

	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("legacy database changed during WAL adoption")
	}
	if got := readFileForTest(t, legacyPath+"-wal"); string(got) != string(walContents) {
		t.Fatalf("legacy WAL sidecar changed during adoption")
	}
	if _, err := os.Stat(legacyPath + "-shm"); err != nil {
		t.Fatalf("legacy SHM sidecar was removed during adoption: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("adopted database missing at %s: %v", newPath, err)
	}
}

func TestProvideRejectsInvalidDefaultLegacyBeforeCreatingCurrent(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	currentPath := filepath.Join(homeDir, "data", "kandev.db")
	legacyContents := []byte("not a sqlite database")
	if err := os.WriteFile(legacyPath, legacyContents, 0o600); err != nil {
		t.Fatalf("seed invalid legacy database: %v", err)
	}

	_, _, err := Provide(&config.Config{HomeDir: homeDir}, nil, "")
	if err == nil {
		t.Fatal("Provide succeeded for an invalid default legacy database")
	}
	if !strings.Contains(err.Error(), legacyPath) {
		t.Fatalf("Provide error = %v, want legacy path %q", err, legacyPath)
	}
	if _, statErr := os.Stat(currentPath); !os.IsNotExist(statErr) {
		t.Fatalf("current database was created after invalid legacy inspection: %v", statErr)
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("invalid legacy database changed during inspection")
	}
}

func TestProvideEstablishedCurrentDatabaseIgnoresInvalidLegacy(t *testing.T) {
	homeDir := t.TempDir()
	currentPath := filepath.Join(homeDir, "data", "kandev.db")
	legacyPath := filepath.Join(homeDir, "kandev.db")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("create current database directory: %v", err)
	}
	seedTaskDatabase(t, currentPath, "current-task", "current task")
	legacyContents := []byte("stale invalid legacy database")
	if err := os.WriteFile(legacyPath, legacyContents, 0o600); err != nil {
		t.Fatalf("seed invalid legacy database: %v", err)
	}

	pool, cleanup, err := Provide(&config.Config{HomeDir: homeDir}, nil, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	var title string
	if err := pool.Reader().Get(&title, `SELECT title FROM tasks WHERE id = 'current-task'`); err != nil {
		t.Fatalf("read established current task: %v", err)
	}
	if title != "current task" {
		t.Fatalf("selected task title = %q, want %q", title, "current task")
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("stale legacy database changed during current startup")
	}
}

func TestProvideFailsClosedWhenEmptyCurrentConflictsWithLegacyHistory(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	currentPath := filepath.Join(homeDir, "data", "kandev.db")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("create current database directory: %v", err)
	}
	currentContents := seedEmptySQLiteDatabase(t, currentPath)
	legacyContents := seedLegacyTaskDatabase(t, legacyPath)

	cfg := &config.Config{HomeDir: homeDir}
	pool, cleanup, err := Provide(cfg, nil, "")
	if err == nil {
		_ = cleanup()
		_ = pool
		t.Fatal("Provide succeeded for an ambiguous SQLite database selection")
	}
	var conflict *SQLiteDatabaseSelectionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Provide error = %v, want SQLiteDatabaseSelectionConflictError", err)
	}
	if !strings.Contains(err.Error(), currentPath) || !strings.Contains(err.Error(), legacyPath) {
		t.Fatalf("Provide error = %v, want both candidate paths", err)
	}

	if got := readFileForTest(t, currentPath); string(got) != string(currentContents) {
		t.Fatalf("current database changed after conflict")
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("legacy database changed after conflict")
	}
}

func TestProvideExplicitDatabasePathBypassesLegacyDiscovery(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	legacyContents := []byte("not a sqlite database")
	if err := os.WriteFile(legacyPath, legacyContents, 0o600); err != nil {
		t.Fatalf("seed invalid legacy database: %v", err)
	}
	explicitPath := filepath.Join(homeDir, "custom", "kandev.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: explicitPath},
		Source: config.ConfigSource{Values: map[string]config.SettingSource{
			"database.path": config.SourceConfiguration,
		}},
		HomeDir: homeDir,
	}

	pool, cleanup, err := Provide(cfg, nil, "")
	if err != nil {
		t.Fatalf("Provide with explicit database path: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if pool.Writer() == nil {
		t.Fatal("Provide returned a nil writer for explicit database path")
	}
	if _, err := os.Stat(explicitPath); err != nil {
		t.Fatalf("explicit database was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "data", "kandev.db")); !os.IsNotExist(err) {
		t.Fatalf("default database was unexpectedly selected: %v", err)
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("legacy candidate changed during explicit-path startup")
	}
}

func TestProvideRetainsCurrentDatabaseWhenBothCandidatesHaveHistory(t *testing.T) {
	homeDir := t.TempDir()
	currentPath := filepath.Join(homeDir, "data", "kandev.db")
	legacyPath := filepath.Join(homeDir, "kandev.db")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("create current database directory: %v", err)
	}
	seedTaskDatabase(t, currentPath, "current-task", "current task")
	legacyContents := seedLegacyTaskDatabase(t, legacyPath)

	cfg := &config.Config{HomeDir: homeDir}
	pool, cleanup, err := Provide(cfg, nil, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	var title string
	if err := pool.Reader().Get(&title, `SELECT title FROM tasks WHERE id = 'current-task'`); err != nil {
		t.Fatalf("read current task: %v", err)
	}
	if title != "current task" {
		t.Fatalf("selected current task title = %q, want %q", title, "current task")
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("legacy database changed while retaining current history")
	}
}

func TestProvideLogsLegacyAdoptionCandidates(t *testing.T) {
	homeDir := t.TempDir()
	currentPath := filepath.Join(homeDir, "data", "kandev.db")
	legacyPath := filepath.Join(homeDir, "kandev.db")
	seedLegacyTaskDatabase(t, legacyPath)

	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	poll, cleanup, err := Provide(&config.Config{HomeDir: homeDir}, log, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	_ = poll

	entries := logs.FilterMessage("SQLite database selected").All()
	if len(entries) != 1 {
		t.Fatalf("selection log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	for field, want := range map[string]any{
		"outcome":            string(sqliteSelectionLegacyAdopted),
		"current_path":       currentPath,
		"legacy_path":        legacyPath,
		"current_task_count": int64(0),
		"legacy_task_count":  int64(1),
	} {
		if got := fields[field]; got != want {
			t.Fatalf("selection log %s = %#v, want %#v", field, got, want)
		}
	}
}

func TestAdoptLegacySQLiteRetainsSourceWhenInstallationFails(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "legacy.db")
	currentPath := filepath.Join(dir, "current.db")
	legacyContents := seedLegacyTaskDatabase(t, legacyPath)
	if err := os.Mkdir(currentPath, 0o700); err != nil {
		t.Fatalf("create installation blocker: %v", err)
	}

	err := adoptLegacySQLite(legacyPath, currentPath, sqliteCandidate{
		path:       legacyPath,
		exists:     true,
		tasksTable: true,
		taskCount:  1,
	})
	if err == nil {
		t.Fatal("adoptLegacySQLite succeeded despite an occupied target directory")
	}
	if got := readFileForTest(t, legacyPath); string(got) != string(legacyContents) {
		t.Fatalf("legacy database changed after failed installation")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read adoption directory: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".kandev-legacy-") {
			t.Fatalf("staged database leaked after failed installation: %s", entry.Name())
		}
	}
}

func TestInstallStagedSQLiteDoesNotReplaceExistingTarget(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "staged.db")
	currentPath := filepath.Join(dir, "current.db")
	if err := os.WriteFile(stagedPath, []byte("staged database"), 0o600); err != nil {
		t.Fatalf("seed staged database: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("late target"), 0o600); err != nil {
		t.Fatalf("seed late target: %v", err)
	}

	err := installStagedSQLite(stagedPath, currentPath)
	if err == nil {
		t.Fatal("installStagedSQLite replaced an existing target")
	}
	if got := readFileForTest(t, currentPath); string(got) != "late target" {
		t.Fatalf("late target changed after failed installation: %q", got)
	}
	if got := readFileForTest(t, stagedPath); string(got) != "staged database" {
		t.Fatalf("staged database changed after rejected installation: %q", got)
	}
}

func seedEmptySQLiteDatabase(t *testing.T, path string) []byte {
	t.Helper()
	database, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open empty database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := ensureMetaTable(database); err != nil {
		_ = database.Close()
		t.Fatalf("create metadata table: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close empty database: %v", err)
	}
	return readFileForTest(t, path)
}

func openWALLegacyTaskDatabase(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open WAL legacy database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	if _, err := database.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatalf("disable WAL autocheckpoint: %v", err)
	}
	if _, err := database.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT NOT NULL)`); err != nil {
		t.Fatalf("create WAL tasks: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO tasks (id, title) VALUES ('task-wal', 'wal task')`); err != nil {
		t.Fatalf("insert WAL task: %v", err)
	}
	return database
}

func readFileForTest(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return contents
}
