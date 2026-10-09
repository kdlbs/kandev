package testutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// SQLiteTemplate builds a schema once and gives every test its own disk-backed
// database. The template is checkpointed and closed before its main file is
// copied, so a clone never depends on the template's WAL or shared state.
type SQLiteTemplate struct {
	initialize func(*sqlx.DB) error
	once       sync.Once
	contents   []byte
	err        error
}

// NewSQLiteTemplate defers schema initialization until the first Open call.
func NewSQLiteTemplate(initialize func(*sqlx.DB) error) *SQLiteTemplate {
	return &SQLiteTemplate{initialize: initialize}
}

// Open returns a writable clone and its path. The caller may reopen the path
// during the test; the connection and directory are cleaned up with t.Cleanup.
func (s *SQLiteTemplate) Open(t testing.TB) (*sqlx.DB, string) {
	t.Helper()
	s.once.Do(func() { s.contents, s.err = s.build(t) })
	if s.err != nil {
		t.Fatalf("build SQLite template: %v", s.err)
	}

	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, s.contents, 0o600); err != nil {
		t.Fatalf("copy SQLite template: %v", err)
	}
	raw, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open SQLite clone: %v", err)
	}
	clone := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() {
		if err := clone.Close(); err != nil {
			t.Errorf("close SQLite clone: %v", err)
		}
	})
	return clone, path
}

func (s *SQLiteTemplate) build(t testing.TB) ([]byte, error) {
	path := filepath.Join(t.TempDir(), "template.db")
	raw, err := db.OpenSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("open template: %w", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	defer func() { _ = database.Close() }()
	if err := s.initialize(database); err != nil {
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	var busy, logPages, checkpointedPages int
	if err := database.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(
		&busy, &logPages, &checkpointedPages,
	); err != nil {
		return nil, fmt.Errorf("checkpoint template: %w", err)
	}
	if busy != 0 {
		return nil, fmt.Errorf("template checkpoint is busy")
	}
	if err := database.Close(); err != nil {
		return nil, fmt.Errorf("close template: %w", err)
	}
	if info, err := os.Stat(path + "-wal"); err == nil {
		if info.Size() != 0 {
			return nil, fmt.Errorf("template WAL still contains %d bytes", info.Size())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat template WAL: %w", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return contents, nil
}
