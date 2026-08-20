package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBaseMigrationsDoNotInferProviderHostFromPartialRepositoryIdentity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repository-provider-identity.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	seedWorkspace(t, repo, "ws-provider-identity")
	repository := &models.Repository{
		ID: "repo-plugin", WorkspaceID: "ws-provider-identity", Name: "widgets", SourceType: "local",
		Provider: "kandev-plugin-tags", RemoteURL: "https://github.com/acme/widgets.git",
	}
	if err := repo.CreateRepository(context.Background(), repository); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	stored, err := repo.GetRepository(context.Background(), repository.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if stored.ProviderHost != "" {
		t.Fatalf("provider_host = %q, want no inferred persistent origin", stored.ProviderHost)
	}
}
