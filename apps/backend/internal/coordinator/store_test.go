package coordinator

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// newTestStore opens a real separate writer pool (single connection, like
// production's db.OpenSQLite) and a separate reader pool (db.OpenSQLiteReader)
// against the same file. A single shared *sqlx.DB for both would self-deadlock
// once the PATCH lock (decision 7) holds BEGIN IMMEDIATE on the one connection
// a nested read would also need.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "coordinator.db")

	writerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = writerConn.Close() })
	writer := sqlx.NewDb(writerConn, "sqlite3")

	readerConn, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = readerConn.Close() })
	reader := sqlx.NewDb(readerConn, "sqlite3")

	store, err := NewStore(writer, reader)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}
