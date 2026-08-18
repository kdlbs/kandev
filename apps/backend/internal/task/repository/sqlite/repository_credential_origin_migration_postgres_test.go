package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresRepositoryCredentialOriginBackfill mirrors
// TestRepositoryCredentialOriginBackfillFillsLegacyAndPluginGitHubRows and
// TestRepositoryCredentialOriginBackfillSkipsAmbiguousAndUnsafeRows against a
// real PostgreSQL schema (AC-10's dialect-portability requirement). Run with
// KANDEV_TEST_POSTGRES_DSN set; otherwise skipped.
func TestPostgresRepositoryCredentialOriginBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-cred-origin", Name: "ws-pg-cred-origin"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	backfillable := &models.Repository{
		ID: "repo-pg-cred-origin-backfillable", WorkspaceID: "ws-pg-cred-origin", Name: "a", SourceType: "local",
		Provider: "kandev-plugin-tags", RemoteURL: "git@github.com:acme/widgets.git",
	}
	skipped := &models.Repository{
		ID: "repo-pg-cred-origin-skipped", WorkspaceID: "ws-pg-cred-origin", Name: "b", SourceType: "local",
		RemoteURL: "git@gitlab.example.com:acme/widgets.git",
	}
	existingHost := &models.Repository{
		ID: "repo-pg-cred-origin-existing", WorkspaceID: "ws-pg-cred-origin", Name: "c", SourceType: "local",
		ProviderHost: "https://ghe.example", RemoteURL: "git@ghe.example:acme/widgets.git",
	}
	for _, repository := range []*models.Repository{backfillable, skipped, existingHost} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("create repository %s: %v", repository.ID, err)
		}
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}

	assertPostgresRepositoryProviderHost(t, db, backfillable.ID, "https://github.com")
	assertPostgresRepositoryProviderHost(t, db, skipped.ID, "")
	assertPostgresRepositoryProviderHost(t, db, existingHost.ID, "https://ghe.example")
}

func assertPostgresRepositoryProviderHost(t *testing.T, db interface {
	Get(dest interface{}, query string, args ...interface{}) error
}, id, want string) {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT provider_host FROM repositories WHERE id = $1`, id); err != nil {
		t.Fatalf("read postgres provider_host %q: %v", id, err)
	}
	if got != want {
		t.Fatalf("postgres provider_host %q = %q, want %q", id, got, want)
	}
}
