package testutil

import (
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

func TestSQLiteTemplateClonesIndependentDurableDatabases(t *testing.T) {
	initialize := func(database *sqlx.DB) error {
		_, err := database.Exec(`CREATE TABLE entries (value TEXT PRIMARY KEY);
			INSERT INTO entries (value) VALUES ('seed')`)
		return err
	}
	template := NewSQLiteTemplate(initialize)

	first, firstPath := template.Open(t)
	if _, err := first.Exec(`INSERT INTO entries (value) VALUES ('first')`); err != nil {
		t.Fatalf("write first clone: %v", err)
	}
	second, secondPath := template.Open(t)
	if firstPath == secondPath {
		t.Fatal("clones share a database path")
	}
	assertEntries(t, second, 1)
	if _, err := second.Exec(`INSERT INTO entries (value) VALUES ('second')`); err != nil {
		t.Fatalf("write second clone: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first clone: %v", err)
	}
	reopened, err := db.OpenSQLite(firstPath)
	if err != nil {
		t.Fatalf("reopen first clone: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertEntries(t, reopened, 2)

	third, _ := template.Open(t)
	assertEntries(t, third, 1)
}

func assertEntries(t *testing.T, database interface{ QueryRow(string, ...any) *sql.Row }, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&got); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
}
