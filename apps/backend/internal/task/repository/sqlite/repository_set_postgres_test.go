package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresRepositorySetRoundTrip is the Postgres counterpart to the SQLite
// repository-set store tests. The set queries use LOWER(name) ordering and
// sqlx.In expansion rather than SQLite-only constructs, and this pins that they
// really do run on Postgres. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRepositorySetRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-sets", Name: "ws-pg-sets"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, id := range []string{"repo-pg-web", "repo-pg-api"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-pg-sets", Name: id, SourceType: "local", LocalPath: "/tmp/" + id,
		}); err != nil {
			t.Fatalf("CreateRepository %s: %v", id, err)
		}
	}

	set := &models.RepositorySet{
		WorkspaceID: "ws-pg-sets",
		Name:        "Full-stack",
		Description: "web + api",
		Items: []models.RepositorySetItem{
			{RepositoryID: "repo-pg-api"},
			{RepositoryID: "repo-pg-web"},
		},
	}
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-pg-api", "repo-pg-web")

	byName, err := repo.GetRepositorySetByName(ctx, "ws-pg-sets", "FULL-stack")
	if err != nil {
		t.Fatalf("GetRepositorySetByName: %v", err)
	}
	if byName == nil || byName.ID != set.ID {
		t.Fatalf("GetRepositorySetByName = %+v", byName)
	}

	pgMembers := []models.RepositorySetItem{{RepositoryID: "repo-pg-web"}}
	if err := repo.UpdateRepositorySet(ctx, set, &pgMembers); err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}
	sets, err := repo.ListRepositorySets(ctx, "ws-pg-sets")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("ListRepositorySets returned %d sets", len(sets))
	}
	assertRepositoryIDs(t, repositoryIDsOf(sets[0]), "repo-pg-web")

	if err := repo.DeleteRepository(ctx, "repo-pg-web"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	loaded, err = repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet after repository delete: %v", err)
	}
	if len(loaded.Items) != 0 {
		t.Fatalf("membership survived repository deletion: %+v", loaded.Items)
	}
}
