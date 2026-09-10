package sqlite

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestCoordinatorGrantSchemaRepairsLegacyForeignKeyAndReplays(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "coordinator-grants.db"))
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
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, "ws-grant", "Grant workspace", now, now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	for _, task := range []struct{ id, workspace string }{{"task-grant", "ws-grant"}, {"task-other", "ws-other"}} {
		if _, err := db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, task.workspace, task.workspace, now, now); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
			t.Fatalf("insert task workspace %s: %v", task.workspace, err)
		}
		if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, task.id, task.workspace, task.id, now, now); err != nil {
			t.Fatalf("insert task %s: %v", task.id, err)
		}
	}

	if _, err := db.Exec(`DROP TABLE workspace_coordinator_grants`); err != nil {
		t.Fatalf("drop current grant table: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (coordinator_task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`); err != nil {
		t.Fatalf("create legacy grant table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO workspace_coordinator_grants VALUES (?, ?, ?, ?, ?)`, "ws-grant", "task-grant", "owner", now, now); err != nil {
		t.Fatalf("insert legacy grant: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("repair legacy grant schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay grant schema migration: %v", err)
	}

	var schema string
	if err := db.Get(&schema, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'workspace_coordinator_grants'`); err != nil {
		t.Fatalf("read grant schema: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(schema), " "))
	for _, fragment := range []string{
		"workspace_id text primary key not null constraint workspace_coordinator_grants_workspace_id_nonempty check (workspace_id <> '')",
		"coordinator_task_id text not null constraint workspace_coordinator_grants_task_id_nonempty check (coordinator_task_id <> '')",
		"foreign key (workspace_id) references workspaces(id) on delete cascade",
		"foreign key (workspace_id, coordinator_task_id) references tasks(workspace_id, id) on delete cascade",
	} {
		if !strings.Contains(normalized, fragment) {
			t.Errorf("grant schema missing %q: %s", fragment, schema)
		}
	}

	if _, err := db.Exec(`INSERT INTO workspace_coordinator_grants VALUES (?, ?, ?, ?, ?)`, "ws-grant", "task-other", "owner", now, now); err == nil {
		t.Fatal("cross-workspace coordinator task was accepted")
	}
	if _, err := db.Exec(`INSERT INTO workspace_coordinator_grants VALUES (?, ?, ?, ?, ?)`, "", "task-grant", "owner", now, now); err == nil {
		t.Fatal("empty workspace grant was accepted")
	}
	if _, err := db.Exec(`INSERT INTO workspace_coordinator_grants VALUES (?, ?, ?, ?, ?)`, "ws-grant", "", "owner", now, now); err == nil {
		t.Fatal("empty coordinator task grant was accepted")
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM workspace_coordinator_grants WHERE workspace_id = 'ws-grant'`); err != nil {
		t.Fatalf("count grant: %v", err)
	}
	if count != 1 {
		t.Fatalf("repaired grant count = %d, want 1", count)
	}
	if _, err := db.Exec(`DELETE FROM tasks WHERE id = ?`, "task-grant"); err != nil {
		t.Fatalf("delete coordinator task: %v", err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM workspace_coordinator_grants WHERE workspace_id = 'ws-grant'`); err != nil {
		t.Fatalf("count cascaded grant: %v", err)
	}
	if count != 0 {
		t.Fatalf("grant count after task cascade = %d, want 0", count)
	}
}
