package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresDesktopDiscoveryRootRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	ctx := context.Background()

	migration := &models.DesktopDiscoveryMigration{HomeConfirmationRequired: true}
	if err := repo.SetDesktopDiscoveryMigration(ctx, migration); err != nil {
		t.Fatalf("set postgres migration: %v", err)
	}
	loadedMigration, err := repo.GetDesktopDiscoveryMigration(ctx)
	if err != nil {
		t.Fatalf("get postgres migration: %v", err)
	}
	if !loadedMigration.HomeConfirmationRequired {
		t.Fatal("postgres migration confirmation was not persisted")
	}

	root := &models.DesktopDiscoveryRoot{
		ID:          "postgres-discovery-root",
		Path:        "/workspace/postgres-code",
		DisplayPath: "~/postgres-code",
		State:       models.DesktopDiscoveryRootConnected,
	}
	if err := repo.CreateDesktopDiscoveryRoot(ctx, root); err != nil {
		t.Fatalf("create postgres discovery root: %v", err)
	}
	loaded, err := repo.GetDesktopDiscoveryRoot(ctx, root.Path)
	if err != nil {
		t.Fatalf("get postgres discovery root: %v", err)
	}
	if loaded == nil || loaded.ID != root.ID {
		t.Fatalf("loaded postgres root = %+v, want id %q", loaded, root.ID)
	}

	loaded.State = models.DesktopDiscoveryRootReconnectRequired
	loaded.LastFailureCode = "permission_denied"
	if err := repo.UpdateDesktopDiscoveryRoot(ctx, loaded); err != nil {
		t.Fatalf("update postgres discovery root: %v", err)
	}
	listed, err := repo.ListDesktopDiscoveryRoots(ctx)
	if err != nil {
		t.Fatalf("list postgres discovery roots: %v", err)
	}
	if len(listed) != 1 || listed[0].State != models.DesktopDiscoveryRootReconnectRequired {
		t.Fatalf("listed postgres roots = %+v", listed)
	}

	if err := repo.DeleteDesktopDiscoveryRoot(ctx, root.Path); err != nil {
		t.Fatalf("delete postgres discovery root: %v", err)
	}
	deleted, err := repo.GetDesktopDiscoveryRoot(ctx, root.Path)
	if err != nil {
		t.Fatalf("get deleted postgres discovery root: %v", err)
	}
	if deleted != nil {
		t.Fatalf("deleted postgres root still exists: %+v", deleted)
	}
}
