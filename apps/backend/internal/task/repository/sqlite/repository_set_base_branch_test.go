package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRepositorySetItemsSchemaIncludesBaseBranch(t *testing.T) {
	repo := newRepoForSetTests(t)

	rows, err := repo.db.Queryx(`PRAGMA table_info(repository_set_items)`)
	if err != nil {
		t.Fatalf("inspect repository_set_items schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan repository_set_items schema: %v", err)
		}
		if name == "base_branch" {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate repository_set_items schema: %v", err)
	}
	t.Fatal("repository_set_items is missing base_branch")
}

func TestRepositorySetRoundTripsSavedBaseBranch(t *testing.T) {
	repo := newRepoForSetTests(t)
	seedWorkspace(t, repo, "ws-base")
	seedSetRepository(t, repo, "ws-base", "repo-base")

	item := models.RepositorySetItem{RepositoryID: "repo-base"}
	baseBranch := reflect.ValueOf(&item).Elem().FieldByName("BaseBranch")
	if !baseBranch.IsValid() {
		t.Fatal("RepositorySetItem is missing BaseBranch")
	}
	baseBranch.SetString("develop")

	set := &models.RepositorySet{
		WorkspaceID: "ws-base",
		Name:        "Base branch",
		Items:       []models.RepositorySetItem{item},
	}
	if err := repo.CreateRepositorySet(context.Background(), set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	loaded, err := repo.GetRepositorySet(context.Background(), set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	got := reflect.ValueOf(loaded.Items[0]).FieldByName("BaseBranch").String()
	if got != "develop" {
		t.Fatalf("saved base branch = %q, want %q", got, "develop")
	}
}

func TestRepositorySetBaseBranchMigrationPreservesLegacyItems(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-legacy-base")
	seedSetRepository(t, repo, "ws-legacy-base", "repo-legacy-base")

	set := &models.RepositorySet{
		WorkspaceID: "ws-legacy-base",
		Name:        "Legacy base",
	}
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
	`), "legacy-item", set.ID, "repo-legacy-base", 0, now, now); err != nil {
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
	if len(loaded.Items) != 1 {
		t.Fatalf("migrated items = %+v, want one item", loaded.Items)
	}
	if loaded.Items[0].RepositoryID != "repo-legacy-base" || loaded.Items[0].BaseBranch != "" {
		t.Fatalf("migrated item = %+v, want legacy membership with empty base", loaded.Items[0])
	}
}
