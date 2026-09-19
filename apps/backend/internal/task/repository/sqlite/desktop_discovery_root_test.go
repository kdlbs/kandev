package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newDesktopDiscoveryTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "desktop-discovery.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("new repository: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo, sqlxDB
}

func TestDesktopDiscoveryRootRoundTrip(t *testing.T) {
	repo, _ := newDesktopDiscoveryTestRepo(t)
	ctx := context.Background()

	root := &models.DesktopDiscoveryRoot{
		ID:          "root-1",
		Path:        "/Users/example/Code",
		DisplayPath: "~/Code",
		State:       models.DesktopDiscoveryRootConnected,
	}
	if err := repo.CreateDesktopDiscoveryRoot(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if root.CreatedAt.IsZero() || root.UpdatedAt.IsZero() {
		t.Fatalf("create timestamps were not populated: %+v", root)
	}

	got, err := repo.GetDesktopDiscoveryRoot(ctx, root.Path)
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if got == nil || got.ID != root.ID || got.DisplayPath != root.DisplayPath {
		t.Fatalf("get root = %+v, want %+v", got, root)
	}

	got.State = models.DesktopDiscoveryRootReconnectRequired
	got.LastFailureCode = "permission_denied"
	if err := repo.UpdateDesktopDiscoveryRoot(ctx, got); err != nil {
		t.Fatalf("update root: %v", err)
	}
	updated, err := repo.GetDesktopDiscoveryRoot(ctx, root.Path)
	if err != nil {
		t.Fatalf("get updated root: %v", err)
	}
	if updated.State != models.DesktopDiscoveryRootReconnectRequired || updated.LastFailureCode != "permission_denied" {
		t.Fatalf("updated root = %+v", updated)
	}

	roots, err := repo.ListDesktopDiscoveryRoots(ctx)
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != root.ID {
		t.Fatalf("listed roots = %+v", roots)
	}
	if err := repo.DeleteDesktopDiscoveryRoot(ctx, root.Path); err != nil {
		t.Fatalf("delete root: %v", err)
	}
	deleted, err := repo.GetDesktopDiscoveryRoot(ctx, root.Path)
	if err != nil {
		t.Fatalf("get deleted root: %v", err)
	}
	if deleted != nil {
		t.Fatalf("deleted root still exists: %+v", deleted)
	}
}

func TestDesktopDiscoveryMigrationRoundTripAndSchemaReplay(t *testing.T) {
	repo, sqlxDB := newDesktopDiscoveryTestRepo(t)
	ctx := context.Background()

	migration, err := repo.GetDesktopDiscoveryMigration(ctx)
	if err != nil {
		t.Fatalf("get migration: %v", err)
	}
	if migration.HomeConfirmationRequired {
		t.Fatal("fresh installation unexpectedly requires Home confirmation")
	}

	migration.HomeConfirmationRequired = true
	if err := repo.SetDesktopDiscoveryMigration(ctx, migration); err != nil {
		t.Fatalf("set migration: %v", err)
	}
	updated, err := repo.GetDesktopDiscoveryMigration(ctx)
	if err != nil {
		t.Fatalf("get updated migration: %v", err)
	}
	if !updated.HomeConfirmationRequired {
		t.Fatal("migration confirmation flag was not persisted")
	}

	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	replayed, err := repo.GetDesktopDiscoveryMigration(ctx)
	if err != nil {
		t.Fatalf("get replayed migration: %v", err)
	}
	if !replayed.HomeConfirmationRequired {
		t.Fatal("schema replay reset migration state")
	}
}
