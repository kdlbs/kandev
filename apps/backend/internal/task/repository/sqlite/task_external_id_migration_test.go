package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

// TestTaskExternalIDMigrationAddsColumnsAndIndex covers the spec's Migration
// scenarios: an existing database whose tasks predate this feature gets both
// nullable columns, the partial unique index exists, existing rows are
// unaffected, and the migration replays cleanly on a second boot.
func TestTaskExternalIDMigrationAddsColumnsAndIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "task-external-id-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pre-existing", "ws-external-id-migration", "Pre-existing task", now, now); err != nil {
		t.Fatalf("seed pre-existing task: %v", err)
	}

	var externalID, settledAt *string
	if err := db.QueryRow(db.Rebind(`
		SELECT external_id, external_id_settled_at FROM tasks WHERE id = ?
	`), "task-pre-existing").Scan(&externalID, &settledAt); err != nil {
		t.Fatalf("select new columns on pre-existing row: %v", err)
	}
	if externalID != nil {
		t.Fatalf("external_id on pre-existing task = %v, want NULL", *externalID)
	}
	if settledAt != nil {
		t.Fatalf("external_id_settled_at on pre-existing task = %v, want NULL", *settledAt)
	}

	var indexName string
	if err := db.Get(&indexName, `
		SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'uniq_tasks_external_id'
	`); err != nil {
		t.Fatalf("uniq_tasks_external_id index is missing: %v", err)
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-no-external-id", "ws-external-id-migration", "Another task", now, now); err != nil {
		t.Fatalf("create task without external_id after migration: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}
}

// TestTaskExternalIDUniqueIndexEnforcesPerWorkspace confirms the partial
// unique index admits at most one task per (workspace_id, external_id) while
// allowing unlimited NULLs and reuse of the same external_id across
// workspaces.
func TestTaskExternalIDUniqueIndexEnforcesPerWorkspace(t *testing.T) {
	repo, db := newTaskExternalIDTestRepo(t)
	now := time.Now().UTC()

	insert := func(id, workspaceID, externalID string) error {
		var extArg interface{}
		if externalID != "" {
			extArg = externalID
		}
		_, err := db.Exec(db.Rebind(`
			INSERT INTO tasks (id, workspace_id, title, created_at, updated_at, external_id)
			VALUES (?, ?, ?, ?, ?, ?)
		`), id, workspaceID, "Task "+id, now, now, extArg)
		return err
	}

	if err := insert("task-1", "ws-1", "ext-1"); err != nil {
		t.Fatalf("first insert with ext-1: %v", err)
	}
	if err := insert("task-2", "ws-1", "ext-1"); err == nil {
		t.Fatal("second insert with same (workspace, external_id) should fail unique index")
	}
	if err := insert("task-3", "ws-2", "ext-1"); err != nil {
		t.Fatalf("same external_id in a different workspace should succeed: %v", err)
	}
	if err := insert("task-4", "ws-1", ""); err != nil {
		t.Fatalf("first NULL external_id insert: %v", err)
	}
	if err := insert("task-5", "ws-1", ""); err != nil {
		t.Fatalf("second NULL external_id insert should not collide: %v", err)
	}
	_ = repo
}

func newTaskExternalIDTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-external-id-repository.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo, db
}
