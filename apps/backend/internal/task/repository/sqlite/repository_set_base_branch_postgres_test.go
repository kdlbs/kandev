package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRepositorySetBaseBranchMigrationPreservesItems(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-base", Name: "ws-pg-base"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-pg-base", WorkspaceID: "ws-pg-base", Name: "repo-pg-base",
		SourceType: "local", LocalPath: "/tmp/repo-pg-base",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	set := &models.RepositorySet{WorkspaceID: "ws-pg-base", Name: "Legacy base"}
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	if _, err := repo.db.Exec(`DROP TABLE repository_set_items`); err != nil {
		t.Fatalf("drop current repository_set_items: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE repository_set_items (
			id TEXT PRIMARY KEY,
			repository_set_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (repository_set_id) REFERENCES repository_sets(id) ON DELETE CASCADE,
			FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
			UNIQUE(repository_set_id, repository_id)
		)
	`); err != nil {
		t.Fatalf("create legacy repository_set_items: %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO repository_set_items
			(id, repository_set_id, repository_id, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "item-pg-base", set.ID, "repo-pg-base", 0, now, now); err != nil {
		t.Fatalf("insert legacy repository-set item: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("run base-branch migration: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay base-branch migration: %v", err)
	}
	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet after migration: %v", err)
	}
	if len(loaded.Items) != 1 || loaded.Items[0].BaseBranch != "" {
		t.Fatalf("migrated item = %+v, want one item with empty base", loaded.Items)
	}
}
