package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCoordinatorGrantSchemaChecksCurrentSchema(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}

	if _, err := db.Exec(`DROP TABLE workspace_coordinator_grants`); err != nil {
		t.Fatalf("drop current coordinator grants: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY NOT NULL CONSTRAINT workspace_coordinator_grants_workspace_id_nonempty CHECK (workspace_id <> ''),
			coordinator_task_id TEXT NOT NULL CONSTRAINT workspace_coordinator_grants_task_id_nonempty CHECK (coordinator_task_id <> ''),
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (coordinator_task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`); err != nil {
		t.Fatalf("create legacy coordinator grants: %v", err)
	}

	otherSchema := fmt.Sprintf("coordinator_grants_other_%d", time.Now().UnixNano())
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE SCHEMA %s;
		CREATE TABLE %s.workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE %s.tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			UNIQUE (workspace_id, id)
		);
		CREATE TABLE %s.workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY NOT NULL,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workspace_id) REFERENCES %s.workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (workspace_id, coordinator_task_id)
				REFERENCES %s.tasks(workspace_id, id) ON DELETE CASCADE
		)`, otherSchema, otherSchema, otherSchema, otherSchema, otherSchema, otherSchema)); err != nil {
		t.Fatalf("create unrelated coordinator grants: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", otherSchema)); err != nil {
			t.Errorf("drop unrelated schema: %v", err)
		}
	})

	current, err := repo.coordinatorGrantSchemaCurrent()
	if err != nil {
		t.Fatalf("check current coordinator grant schema: %v", err)
	}
	if current {
		t.Fatal("legacy current schema was accepted because another schema has the corrected foreign key")
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("repair current coordinator grant schema: %v", err)
	}
	current, err = repo.coordinatorGrantSchemaCurrent()
	if err != nil {
		t.Fatalf("recheck repaired coordinator grant schema: %v", err)
	}
	if !current {
		t.Fatal("repaired current coordinator grant schema was not accepted")
	}
}
