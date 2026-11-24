package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/prompts/models"

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
	if released, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{}); err != nil || released != 1 {
		t.Fatalf("release synced prompts = %d, %v", released, err)
	}
	if deleted, err := repo.DeleteSyncedPrompt(ctx, created.ID); err != nil || deleted {
		t.Fatalf("delete released prompt = %v, %v", deleted, err)
	}
}

func TestPostgresRepository_PromptCountLimitHoldsUnderConcurrentCreates(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize prompts repository: %v", err)
	}
	ctx := context.Background()
	var existing int
	if err := database.GetContext(ctx, &existing, `SELECT COUNT(*) FROM custom_prompts`); err != nil {
		t.Fatal(err)
	}
	for i := existing; i < maxPromptListItems-1; i++ {
		if err := repo.CreatePrompt(ctx, &models.Prompt{Name: fmt.Sprintf("seed-%d", i), Content: "x"}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	errs := make(chan error, 2)
	go func() { errs <- repo.CreatePrompt(ctx, &models.Prompt{Name: "race-manual", Content: "x"}) }()
	go func() { _, err := repo.CreateSyncedPrompt(ctx, "race-synced", "x", "p/race.md"); errs <- err }()
	<-errs
	<-errs
	var total int
	if err := database.GetContext(ctx, &total, `SELECT COUNT(*) FROM custom_prompts`); err != nil {
		t.Fatal(err)
	}
	if total > maxPromptListItems {
		t.Fatalf("prompt count = %d, want at most %d", total, maxPromptListItems)
	}
}
