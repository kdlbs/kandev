package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

func createService(t *testing.T) (*Service, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repoImpl, repoCleanup, err := promptstore.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	cleanup := func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
		if err := repoCleanup(); err != nil {
			t.Errorf("close repo: %v", err)
		}
	}
	return NewService(repoImpl), cleanup
}

func TestService_CreatePromptValidation(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreatePrompt(ctx, "", "content"); err != ErrInvalidPrompt {
		t.Fatalf("expected invalid prompt error, got %v", err)
	}
	if _, err := svc.CreatePrompt(ctx, "name", ""); err != ErrInvalidPrompt {
		t.Fatalf("expected invalid prompt error, got %v", err)
	}
}

func TestService_UpdatePrompt(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	prompt, err := svc.CreatePrompt(ctx, "Morning", "Hello")
	if err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	name := "Evening"
	content := "Goodbye"
	updated, err := svc.UpdatePrompt(ctx, prompt.ID, &name, &content)
	if err != nil {
		t.Fatalf("update prompt: %v", err)
	}
	if updated.Name != name {
		t.Fatalf("expected name %q, got %q", name, updated.Name)
	}
	if updated.Content != content {
		t.Fatalf("expected content %q, got %q", content, updated.Content)
	}
}

func TestService_CreatePromptDuplicateName(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreatePrompt(ctx, "shared-name", "first"); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}
	if _, err := svc.CreatePrompt(ctx, "shared-name", "second"); err != ErrPromptAlreadyExists {
		t.Fatalf("expected ErrPromptAlreadyExists, got %v", err)
	}
	// Trimmed input still detected.
	if _, err := svc.CreatePrompt(ctx, "  shared-name  ", "third"); err != ErrPromptAlreadyExists {
		t.Fatalf("expected ErrPromptAlreadyExists for trimmed name, got %v", err)
	}
}

func TestService_UpdatePromptRenameToExisting(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreatePrompt(ctx, "alpha", "a"); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	beta, err := svc.CreatePrompt(ctx, "beta", "b")
	if err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	rename := "alpha"
	if _, err := svc.UpdatePrompt(ctx, beta.ID, &rename, nil); err != ErrPromptAlreadyExists {
		t.Fatalf("expected ErrPromptAlreadyExists, got %v", err)
	}
}

// Saving a prompt without changing its name (e.g. content-only edit, or sending
// the same name through the PATCH) must not trip the duplicate-name guard.
func TestService_UpdatePromptSameName(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	prompt, err := svc.CreatePrompt(ctx, "stable", "v1")
	if err != nil {
		t.Fatalf("seed prompt: %v", err)
	}

	sameName := "stable"
	newContent := "v2"
	updated, err := svc.UpdatePrompt(ctx, prompt.ID, &sameName, &newContent)
	if err != nil {
		t.Fatalf("update with same name: %v", err)
	}
	if updated.Content != newContent {
		t.Fatalf("expected content %q, got %q", newContent, updated.Content)
	}
}
