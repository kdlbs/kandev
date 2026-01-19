package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/prompts/models"
)

func createTestRepo(t *testing.T) (*sqliteRepository, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	repo, err := newSQLiteRepositoryWithDB(dbConn)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	cleanup := func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := repo.Close(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	}
	return repo, cleanup
}

func TestSQLiteRepository_CRUD(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()

	prompt := &models.Prompt{Name: "Daily Summary", Content: "Summarize the work."}
	if err := repo.CreatePrompt(ctx, prompt); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	if prompt.ID == "" {
		t.Fatalf("expected id to be set")
	}

	fetched, err := repo.GetPromptByID(ctx, prompt.ID)
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if fetched.Name != prompt.Name {
		t.Fatalf("expected name %q, got %q", prompt.Name, fetched.Name)
	}

	fetchedByName, err := repo.GetPromptByName(ctx, prompt.Name)
	if err != nil {
		t.Fatalf("get prompt by name: %v", err)
	}
	if fetchedByName.ID != prompt.ID {
		t.Fatalf("expected prompt id %q, got %q", prompt.ID, fetchedByName.ID)
	}

	prompt.Name = "Standup"
	prompt.Content = "What did you do yesterday?"
	if err := repo.UpdatePrompt(ctx, prompt); err != nil {
		t.Fatalf("update prompt: %v", err)
	}

	list, err := repo.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(list))
	}
	if list[0].Name != "Standup" {
		t.Fatalf("expected updated name, got %q", list[0].Name)
	}

	if err := repo.DeletePrompt(ctx, prompt.ID); err != nil {
		t.Fatalf("delete prompt: %v", err)
	}

	list, err = repo.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("list prompts after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no prompts after delete, got %d", len(list))
	}
}
