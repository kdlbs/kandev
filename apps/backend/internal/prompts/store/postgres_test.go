package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRepository_ListPromptsForReferenceExpansion(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize prompts repository: %v", err)
	}

	prompts, truncated, err := repo.ListPromptsForReferenceExpansion(
		context.Background(), 20, 512, 1<<20, 1<<20, 16<<20,
	)
	if err != nil {
		t.Fatalf("list prompts for reference expansion: %v", err)
	}
	if truncated {
		t.Fatal("expected seeded prompts to fit within the candidate limit")
	}
	if len(prompts) == 0 {
		t.Fatal("expected seeded prompts")
	}
}

func TestPostgresRepository_SyncedPromptOwnership(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize prompts repository: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(database, database); err != nil {
		t.Fatalf("replay prompts schema: %v", err)
	}
	ctx := context.Background()
	created, err := repo.CreateSyncedPrompt(ctx, "pg-synced", "Repo", "p/pg-synced.md")
	if err != nil {
		t.Fatalf("create synced prompt: %v", err)
	}
	read, err := repo.GetPromptByID(ctx, created.ID)
	if err != nil || !read.IsSynced() || read.SourcePath != "p/pg-synced.md" {
		t.Fatalf("read synced prompt = %+v, %v", read, err)
	}
	if changed, err := repo.UpdateSyncedPrompt(ctx, created.ID, "Repo 2", "p/pg-synced.md"); err != nil || !changed {
		t.Fatalf("update synced prompt = %v, %v", changed, err)
	}
	if released, err := repo.ReleaseSyncedPrompts(ctx); err != nil || released != 1 {
		t.Fatalf("release synced prompts = %d, %v", released, err)
	}
	if deleted, err := repo.DeleteSyncedPrompt(ctx, created.ID); err != nil || deleted {
		t.Fatalf("delete released prompt = %v, %v", deleted, err)
	}
}
