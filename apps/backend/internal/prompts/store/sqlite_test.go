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
	// Should have 1 custom prompt + 3 built-in prompts
	if len(list) < 1 {
		t.Fatalf("expected at least 1 prompt, got %d", len(list))
	}
	// Find our custom prompt (built-in prompts come first due to ORDER BY)
	var found bool
	for _, p := range list {
		if p.ID == prompt.ID && p.Name == "Standup" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find updated prompt with name 'Standup'")
	}

	if err := repo.DeletePrompt(ctx, prompt.ID); err != nil {
		t.Fatalf("delete prompt: %v", err)
	}

	list, err = repo.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("list prompts after delete: %v", err)
	}
	// Should only have built-in prompts left (3)
	builtinCount := 0
	for _, p := range list {
		if p.Builtin {
			builtinCount++
		}
		if p.ID == prompt.ID {
			t.Fatalf("expected custom prompt to be deleted, but it still exists")
		}
	}
	if builtinCount != 3 {
		t.Fatalf("expected 3 built-in prompts, got %d", builtinCount)
	}
}

func TestSQLiteRepository_BuiltinPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()

	// List prompts should include built-in prompts
	list, err := repo.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}

	// Should have 3 built-in prompts
	builtinCount := 0
	for _, p := range list {
		if p.Builtin {
			builtinCount++
		}
	}

	if builtinCount != 3 {
		t.Fatalf("expected 3 built-in prompts, got %d", builtinCount)
	}
}
