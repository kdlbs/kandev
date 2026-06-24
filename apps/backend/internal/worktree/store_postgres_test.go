package worktree

import (
	"testing"

	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStore_ReinitializesSchema(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first task schema init: %v", err)
	}
	if _, err := NewSQLiteStore(db, db); err != nil {
		t.Fatalf("first worktree schema init: %v", err)
	}
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second task schema init: %v", err)
	}
	if _, err := NewSQLiteStore(db, db); err != nil {
		t.Fatalf("second worktree schema init: %v", err)
	}
}
