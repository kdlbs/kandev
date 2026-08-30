package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func TestE2EResetDeletesWorkspaceGitHubAuthentication(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE github_workspace_connections (workspace_id TEXT PRIMARY KEY);
		CREATE TABLE github_user_connections (workspace_id TEXT, user_id TEXT);
		CREATE TABLE github_auth_flows (state_hash TEXT PRIMARY KEY, workspace_id TEXT);
		CREATE TABLE secrets (id TEXT PRIMARY KEY);
		INSERT INTO github_workspace_connections VALUES ('ws-1'), ('ws-2');
		INSERT INTO github_user_connections VALUES ('ws-1', 'user-1'), ('ws-2', 'user-2');
		INSERT INTO github_auth_flows VALUES ('state-1', 'ws-1'), ('state-2', 'ws-2');
		INSERT INTO secrets VALUES
			('github:workspace:ws-1:pat'),
			('github:user:ws-1:user-1:access'),
			('github:user:ws-1:user-1:refresh'),
			('github:workspace:ws-2:pat')
	`); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	if err := deleteGitHubAuthForReset(context.Background(), database.DB, "ws-1"); err != nil {
		t.Fatalf("delete GitHub auth: %v", err)
	}
	for _, table := range []string{
		"github_workspace_connections", "github_user_connections", "github_auth_flows",
	} {
		assertWorkspaceRows(t, database, table, "ws-1", 0)
		assertWorkspaceRows(t, database, table, "ws-2", 1)
	}
	var ws1Secrets, ws2Secrets int
	if err := database.Get(&ws1Secrets,
		`SELECT COUNT(*) FROM secrets WHERE id = ? OR substr(id, 1, length(?)) = ?`,
		"github:workspace:ws-1:pat", "github:user:ws-1:", "github:user:ws-1:"); err != nil {
		t.Fatalf("count deleted secrets: %v", err)
	}
	if err := database.Get(&ws2Secrets,
		`SELECT COUNT(*) FROM secrets WHERE id = ? OR substr(id, 1, length(?)) = ?`,
		"github:workspace:ws-2:pat", "github:user:ws-2:", "github:user:ws-2:"); err != nil {
		t.Fatalf("count preserved secrets: %v", err)
	}
	if ws1Secrets != 0 || ws2Secrets != 1 {
		t.Fatalf("secret counts = ws-1:%d ws-2:%d, want 0 and 1", ws1Secrets, ws2Secrets)
	}
}

func TestE2EResetDeletesWorkspaceCIRunStateInDependencyOrder(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset-ci-runs.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE github_ci_run_grants (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL
		);
		CREATE TABLE github_ci_run_requests (
			id TEXT PRIMARY KEY,
			grant_id TEXT NOT NULL REFERENCES github_ci_run_grants(id),
			workspace_id TEXT NOT NULL
		);
		CREATE TABLE github_ci_run_audit_events (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL REFERENCES github_ci_run_requests(id)
		);
		INSERT INTO github_ci_run_grants VALUES ('grant-1', 'ws-1'), ('grant-2', 'ws-2');
		INSERT INTO github_ci_run_requests VALUES
			('request-1', 'grant-1', 'ws-1'),
			('request-2', 'grant-2', 'ws-2');
		INSERT INTO github_ci_run_audit_events VALUES
			('audit-1', 'request-1'),
			('audit-2', 'request-2');
	`); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	if err := deleteGitHubCIRunStateForReset(context.Background(), database.DB, "ws-1"); err != nil {
		t.Fatalf("delete CI run state: %v", err)
	}
	assertWorkspaceRows(t, database, "github_ci_run_grants", "ws-1", 0)
	assertWorkspaceRows(t, database, "github_ci_run_grants", "ws-2", 1)
	assertWorkspaceRows(t, database, "github_ci_run_requests", "ws-1", 0)
	assertWorkspaceRows(t, database, "github_ci_run_requests", "ws-2", 1)
	for _, tc := range []struct {
		id   string
		want int
	}{{id: "audit-1", want: 0}, {id: "audit-2", want: 1}} {
		var got int
		if err := database.Get(&got,
			`SELECT COUNT(*) FROM github_ci_run_audit_events WHERE id = ?`, tc.id); err != nil {
			t.Fatalf("count %s: %v", tc.id, err)
		}
		if got != tc.want {
			t.Fatalf("%s rows = %d, want %d", tc.id, got, tc.want)
		}
	}
}

func assertWorkspaceRows(t *testing.T, database *sqlx.DB, table, workspaceID string, want int) {
	t.Helper()
	var got int
	if err := database.Get(&got, `SELECT COUNT(*) FROM `+table+` WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows for %s = %d, want %d", table, workspaceID, got, want)
	}
}
