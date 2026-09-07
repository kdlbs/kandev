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
