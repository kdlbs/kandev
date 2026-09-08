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
