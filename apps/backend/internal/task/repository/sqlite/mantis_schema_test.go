package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// newRepoForMantisSchemaTests opens a fresh on-disk SQLite repo. On-disk
// (rather than :memory:) because the migration regression we care about —
// running initSchema twice in the same process — exercises the persistence
// layer the same way a backend restart does.
func newRepoForMantisSchemaTests(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo, sqlxDB
}

// TestInitMantisSchema_FreshDBCreatesTables locks in the contract that a fresh
// install gets all three Mantis tables plus the workspace lookup index. If a
// later refactor accidentally removes a CREATE TABLE / CREATE INDEX, this test
// catches it before the orchestrator wiring in T03 hits a "no such table"
// error at runtime.
func TestInitMantisSchema_FreshDBCreatesTables(t *testing.T) {
	_, db := newRepoForMantisSchemaTests(t)

	expectTable(t, db, "mantis_configs")
	expectTable(t, db, "mantis_issue_watches")
	expectTable(t, db, "mantis_issue_watch_tasks")
	expectIndex(t, db, "idx_mantis_issue_watches_workspace")
}

// TestInitMantisSchema_Idempotent locks in the no-op-safe contract: calling
// initMantisSchema twice (the boot path + the runMigrations re-application)
// must not fail on a database that already has the tables. Acceptance
// criterion: "Migration ist auf frischer und auf existierender DB no-op-safe;
// zweimal hintereinander migrieren wirft keinen Fehler".
func TestInitMantisSchema_Idempotent(t *testing.T) {
	repo, _ := newRepoForMantisSchemaTests(t)

	if err := repo.initMantisSchema(); err != nil {
		t.Fatalf("second initMantisSchema call: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations re-invocation: %v", err)
	}
}

// TestInitMantisSchema_InsertRoundTrip proves the tables aren't just empty
// shells: a real insert against mantis_configs and mantis_issue_watches
// succeeds with the columns declared in mantisSchemaDDL. Catches column-name
// drift between models.go and the DDL before the service layer in T03 starts
// depending on the schema.
func TestInitMantisSchema_InsertRoundTrip(t *testing.T) {
	_, db := newRepoForMantisSchemaTests(t)
	now := time.Now().UTC()
	workspaceID := uuid.New().String()

	if _, err := db.Exec(`
		INSERT INTO mantis_configs (
			workspace_id, base_url, username, auth_method, default_project_id,
			last_ok, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, "https://example.mantis", "user", "api_token", 0, 0, "", now, now,
	); err != nil {
		t.Fatalf("insert mantis_configs: %v", err)
	}

	watchID := uuid.New().String()
	if _, err := db.Exec(`
		INSERT INTO mantis_issue_watches (
			id, workspace_id, workflow_id, workflow_step_id, filter,
			agent_profile_id, executor_profile_id, prompt, enabled,
			poll_interval_seconds, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		watchID, workspaceID, "wf", "step", "{}", "", "", "", 1, 300, "", now, now,
	); err != nil {
		t.Fatalf("insert mantis_issue_watches: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO mantis_issue_watch_tasks (
			id, issue_watch_id, issue_id, issue_url, task_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), watchID, "1234", "https://example.mantis/view.php?id=1234", "", now,
	); err != nil {
		t.Fatalf("insert mantis_issue_watch_tasks: %v", err)
	}
}

// expectTable fails the test when the named table is not in sqlite_master.
func expectTable(t *testing.T, db *sqlx.DB, name string) {
	t.Helper()
	var found string
	err := db.Get(&found,
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name)
	if err != nil {
		t.Fatalf("query sqlite_master for %s: %v", name, err)
	}
	if found != name {
		t.Fatalf("expected table %q, got %q", name, found)
	}
}

// expectIndex fails the test when the named index is not in sqlite_master.
func expectIndex(t *testing.T, db *sqlx.DB, name string) {
	t.Helper()
	var found string
	err := db.Get(&found,
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name)
	if err != nil {
		t.Fatalf("query sqlite_master for index %s: %v", name, err)
	}
	if found != name {
		t.Fatalf("expected index %q, got %q", name, found)
	}
}
