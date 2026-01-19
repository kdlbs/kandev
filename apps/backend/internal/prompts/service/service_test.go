package service

import (
	"context"
	"path/filepath"
	"testing"

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
	repoImpl, repoCleanup, err := promptstore.Provide(dbConn)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	cleanup := func() {
		if err := dbConn.Close(); err != nil {
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
