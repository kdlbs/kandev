package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestSQLiteSchemaReinitializes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema-replay.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("first schema init: %v", err)
	}
	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("second schema init: %v", err)
	}
}
