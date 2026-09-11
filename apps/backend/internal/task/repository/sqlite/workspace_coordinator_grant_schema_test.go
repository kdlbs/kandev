package sqlite

import (
	"testing"
	"time"
)

func TestWorkspaceCoordinatorGrantSchemaRejectsCrossWorkspaceDesignation(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES
			('workspace-a', 'A', ?, ?), ('workspace-b', 'B', ?, ?)
	`), now, now, now, now); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('coordinator-a', 'workspace-a', 'Coordinator', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed coordinator task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-a', 'coordinator-a', 'operator', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("insert same-workspace coordinator designation: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-b', 'coordinator-a', 'operator', ?, ?)
	`), now, now); err == nil {
		t.Fatal("cross-workspace coordinator designation succeeded")
	}
}

func TestWorkspaceCoordinatorGrantFreshSchemaAllowsTaskMutation(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	now := time.Now().UTC()
	seedWorkspaceCoordinatorGrantTask(t, repo, now)

	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-a', 'coordinator-a', 'operator', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("insert coordinator designation: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE tasks SET title = 'Updated coordinator', updated_at = ? WHERE id = 'coordinator-a'
	`), now); err != nil {
		t.Fatalf("update task with coordinator designation: %v", err)
	}
}

func TestWorkspaceCoordinatorGrantMigrationRepairsLegacyForeignKeyBeforeTaskMutation(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	now := time.Now().UTC()
	seedWorkspaceCoordinatorGrantTask(t, repo, now)

	if _, err := repo.db.Exec(`DROP TABLE workspace_coordinator_grants`); err != nil {
		t.Fatalf("drop current coordinator designation schema: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT NOT NULL PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (coordinator_task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create legacy coordinator designation schema: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
		VALUES ('workspace-a', 'coordinator-a', 'operator', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed legacy coordinator designation: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy coordinator designation schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay legacy coordinator designation migration: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE tasks SET title = 'Updated coordinator', updated_at = ? WHERE id = 'coordinator-a'
	`), now); err != nil {
		t.Fatalf("update task after coordinator designation migration: %v", err)
	}
}

func seedWorkspaceCoordinatorGrantTask(t *testing.T, repo *Repository, now time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at)
		VALUES ('workspace-a', 'A', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('coordinator-a', 'workspace-a', 'Coordinator', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed coordinator task: %v", err)
	}
}
