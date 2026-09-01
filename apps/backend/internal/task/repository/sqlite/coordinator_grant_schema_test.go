package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestWorkspaceCoordinatorGrantSchemaIsReplaySafeAndWorkspaceBound(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertWorkspaceCoordinatorGrantSchema(t, repo)
}

func TestWorkspaceCoordinatorGrantSchemaMigratesLegacySingleTaskForeignKey(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertLegacyWorkspaceCoordinatorGrantSchemaMigration(t, repo)
}

func TestPostgresWorkspaceCoordinatorGrantSchemaMigratesLegacySingleTaskForeignKey(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres coordinator grant schema: %v", err)
	}
	assertLegacyWorkspaceCoordinatorGrantSchemaMigration(t, repo)
}

func assertLegacyWorkspaceCoordinatorGrantSchemaMigration(t *testing.T, repo *Repository) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES
			('workspace-a', 'A', ?, ?), ('workspace-b', 'B', ?, ?)
	`), now, now, now, now); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES
			('coordinator-a', 'workspace-a', 'Coordinator A', ?, ?),
			('coordinator-b', 'workspace-a', 'Coordinator B', ?, ?)
	`), now, now, now, now); err != nil {
		t.Fatalf("seed coordinator task: %v", err)
	}
	if _, err := repo.db.Exec(`DROP TABLE workspace_coordinator_grants`); err != nil {
		t.Fatalf("drop current coordinator grant schema: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (coordinator_task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create legacy coordinator grant schema: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-b', 'coordinator-a', 'owner-b', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed legacy cross-workspace grant: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-a', 'coordinator-a', 'owner-a', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed legacy same-workspace grant: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy coordinator grant schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrated coordinator grant schema: %v", err)
	}
	var migratedRows int
	if err := repo.db.Get(&migratedRows, `SELECT COUNT(*) FROM workspace_coordinator_grants`); err != nil {
		t.Fatalf("count migrated coordinator grants: %v", err)
	}
	if migratedRows != 1 {
		t.Fatalf("migration retained the wrong coordinator grant rows: %d", migratedRows)
	}
	var coordinatorTaskID string
	if err := repo.db.Get(&coordinatorTaskID, `
		SELECT coordinator_task_id FROM workspace_coordinator_grants WHERE workspace_id = 'workspace-a'
	`); err != nil {
		t.Fatalf("read preserved coordinator grant: %v", err)
	}
	if coordinatorTaskID != "coordinator-a" {
		t.Fatalf("migration changed same-workspace coordinator grant task: %q", coordinatorTaskID)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-b', 'coordinator-b', 'owner-b', ?, ?)
	`), now, now); err == nil {
		t.Fatal("migrated coordinator grant schema accepted a cross-workspace pair")
	}
}

func TestPostgresWorkspaceCoordinatorGrantSchemaIsReplaySafeAndWorkspaceBound(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres coordinator grant schema: %v", err)
	}
	assertWorkspaceCoordinatorGrantSchema(t, repo)
}

func assertWorkspaceCoordinatorGrantSchema(t *testing.T, repo *Repository) {
	t.Helper()
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("first migration replay: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("second migration replay: %v", err)
	}

	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES
			('', 'Empty', ?, ?), ('workspace-a', 'A', ?, ?),
			('workspace-b', 'B', ?, ?), ('workspace-c', 'C', ?, ?)
	`), now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES
			('coordinator-a', 'workspace-a', 'Coordinator A', ?, ?),
			('coordinator-empty-workspace', '', 'Empty workspace', ?, ?),
			('', 'workspace-c', 'Empty task ID', ?, ?)
	`), now, now, now, now, now, now); err != nil {
		t.Fatalf("seed coordinator task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-a', 'coordinator-a', 'owner-a', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("insert same-workspace grant: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-b', 'coordinator-a', 'owner-b', ?, ?)
	`), now, now); err == nil {
		t.Fatal("cross-workspace Coordinator grant was representable")
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('', 'coordinator-empty-workspace', 'owner-empty', ?, ?)
	`), now, now); err == nil {
		t.Fatal("empty-workspace Coordinator grant was representable")
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-c', '', 'owner-c', ?, ?)
	`), now, now); err == nil {
		t.Fatal("empty-task Coordinator grant was representable")
	}
	if _, err := repo.db.Exec(`DELETE FROM tasks WHERE id = 'coordinator-a'`); err != nil {
		t.Fatalf("delete coordinator task: %v", err)
	}
	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM workspace_coordinator_grants`); err != nil {
		t.Fatalf("count grant after task delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("coordinator task delete retained %d grant rows", count)
	}
}
