package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestDeleteMantisStateForReset_OnlyTargetWorkspace locks in the workspace
// isolation contract: the cleanup deletes Mantis state for one workspace and
// leaves every other workspace untouched. Catches accidental over-broad
// DELETEs (e.g. dropping the WHERE clause) before they leak into specs where
// two workspaces share a Playwright worker's database.
func TestDeleteMantisStateForReset_OnlyTargetWorkspace(t *testing.T) {
	repo, sqlxDB := newE2EResetTestRepo(t)
	_ = repo

	target := uuid.New().String()
	bystander := uuid.New().String()
	seedMantisFixtures(t, sqlxDB, target)
	seedMantisFixtures(t, sqlxDB, bystander)

	if err := deleteMantisStateForReset(context.Background(), sqlxDB.DB, target); err != nil {
		t.Fatalf("deleteMantisStateForReset: %v", err)
	}

	if got := countMantisConfigs(t, sqlxDB, target); got != 0 {
		t.Errorf("target mantis_configs still has %d rows, want 0", got)
	}
	if got := countMantisWatches(t, sqlxDB, target); got != 0 {
		t.Errorf("target mantis_issue_watches still has %d rows, want 0", got)
	}
	if got := countMantisWatchTasks(t, sqlxDB, target); got != 0 {
		t.Errorf("target mantis_issue_watch_tasks still has %d rows, want 0", got)
	}

	if got := countMantisConfigs(t, sqlxDB, bystander); got != 1 {
		t.Errorf("bystander mantis_configs has %d rows, want 1 (cleanup leaked across workspaces)", got)
	}
	if got := countMantisWatches(t, sqlxDB, bystander); got != 1 {
		t.Errorf("bystander mantis_issue_watches has %d rows, want 1", got)
	}
	if got := countMantisWatchTasks(t, sqlxDB, bystander); got != 1 {
		t.Errorf("bystander mantis_issue_watch_tasks has %d rows, want 1", got)
	}
}

// TestDeleteMantisStateForReset_Idempotent locks in the no-op-safe contract:
// running the cleanup against a workspace that has no Mantis rows must not
// error. Reset endpoints are hit between every spec, so the empty-state path
// is hot.
func TestDeleteMantisStateForReset_Idempotent(t *testing.T) {
	_, sqlxDB := newE2EResetTestRepo(t)
	ws := uuid.New().String()
	if err := deleteMantisStateForReset(context.Background(), sqlxDB.DB, ws); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := deleteMantisStateForReset(context.Background(), sqlxDB.DB, ws); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// newE2EResetTestRepo opens an on-disk SQLite repo so initSchema runs through
// the same code path the live binary uses on boot.
func newE2EResetTestRepo(t *testing.T) (*sqliterepo.Repository, *sqlx.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo, sqlxDB
}

// seedMantisFixtures writes one row into each of the three Mantis tables for
// the given workspace, with a deterministic watch ID chain so the watch_tasks
// row's FK to mantis_issue_watches is satisfied.
func seedMantisFixtures(t *testing.T, db *sqlx.DB, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO mantis_configs (
			workspace_id, base_url, username, auth_method, default_project_id,
			last_ok, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, "https://example.mantis", "user", "api_token", 0, 0, "", now, now,
	); err != nil {
		t.Fatalf("seed mantis_configs: %v", err)
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
		t.Fatalf("seed mantis_issue_watches: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO mantis_issue_watch_tasks (
			id, issue_watch_id, issue_id, issue_url, task_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), watchID, "42", "https://example.mantis/view.php?id=42", "", now,
	); err != nil {
		t.Fatalf("seed mantis_issue_watch_tasks: %v", err)
	}
}

func countMantisConfigs(t *testing.T, db *sqlx.DB, workspaceID string) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM mantis_configs WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("count mantis_configs: %v", err)
	}
	return n
}

func countMantisWatches(t *testing.T, db *sqlx.DB, workspaceID string) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM mantis_issue_watches WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("count mantis_issue_watches: %v", err)
	}
	return n
}

func countMantisWatchTasks(t *testing.T, db *sqlx.DB, workspaceID string) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `
		SELECT COUNT(*) FROM mantis_issue_watch_tasks
		WHERE issue_watch_id IN (SELECT id FROM mantis_issue_watches WHERE workspace_id = ?)
	`, workspaceID); err != nil {
		t.Fatalf("count mantis_issue_watch_tasks: %v", err)
	}
	return n
}
