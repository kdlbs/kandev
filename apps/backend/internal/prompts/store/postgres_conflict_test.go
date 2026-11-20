package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/prompts/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRepository_OwnershipTransitionsKeepAlternateContent(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize prompts repository: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(database, database); err != nil {
		t.Fatalf("replay prompts schema: %v", err)
	}
	ctx := context.Background()
	manual := &models.Prompt{Name: "pg-conflict", Content: "Mine"}
	if err := repo.CreatePrompt(ctx, manual); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	steps := []struct {
		name string
		run  func() (bool, error)
	}{
		{"adopt as override", func() (bool, error) { return repo.AdoptPromptAsOverride(ctx, manual.ID, "p/pg.md", "Repo", true) }},
		{"clear conflict", func() (bool, error) { return repo.ClearOverrideConflict(ctx, manual.ID) }},
		{"revert", func() (bool, error) { return repo.RevertOverride(ctx, manual.ID) }},
		{"sync update", func() (bool, error) { return repo.UpdateSyncedPrompt(ctx, manual.ID, "Repo", "p/pg.md") }},
		{"override", func() (bool, error) { return repo.OverrideSyncedPrompt(ctx, manual.ID) }},
	}
	for _, step := range steps {
		if changed, err := step.run(); err != nil || !changed {
			t.Fatalf("%s = %v, %v", step.name, changed, err)
		}
	}
	read, err := repo.GetPromptByID(ctx, manual.ID)
	if err != nil || !read.IsOverride() || read.Content != "Mine" || read.AlternateContent != "Repo" || read.Conflict {
		t.Fatalf("restored override = %+v, %v", read, err)
	}
	other := &models.Prompt{Name: "pg-taken", Content: "Local"}
	if err := repo.CreatePrompt(ctx, other); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	if changed, err := repo.AdoptPromptAsSynced(ctx, other.ID, "Repo", "p/taken.md"); err != nil || !changed {
		t.Fatalf("adopt as synced = %v, %v", changed, err)
	}
	if changed, err := repo.RestoreSyncedPrompt(ctx, other.ID); err != nil || !changed {
		t.Fatalf("restore = %v, %v", changed, err)
	}
	restored, err := repo.GetPromptByID(ctx, other.ID)
	if err != nil || restored.Source != models.PromptSourceManual || restored.Content != "Local" {
		t.Fatalf("restored manual = %+v, %v", restored, err)
	}
}

func TestPostgresRepository_ReleaseRestoresChosenKeptVersions(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize prompts repository: %v", err)
	}
	ctx := context.Background()
	prompt := &models.Prompt{Name: "pg-restore", Content: "Local"}
	if err := repo.CreatePrompt(ctx, prompt); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	if changed, err := repo.AdoptPromptAsSynced(ctx, prompt.ID, "Repo", "p/pg-restore.md"); err != nil || !changed {
		t.Fatalf("adopt as synced = %v, %v", changed, err)
	}
	if released, err := repo.ReleaseSyncedPrompts(ctx, "pg-restore"); err != nil || released != 1 {
		t.Fatalf("release = %d, %v", released, err)
	}
	read, err := repo.GetPromptByID(ctx, prompt.ID)
	if err != nil || read.Source != models.PromptSourceManual || read.Content != "Local" || read.AlternateContent != "" {
		t.Fatalf("released prompt = %+v, %v", read, err)
	}
}
