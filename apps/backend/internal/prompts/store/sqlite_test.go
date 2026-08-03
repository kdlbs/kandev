package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	promptcfg "github.com/kandev/kandev/config/prompts"
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
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	cleanup := func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := repo.Close(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	}
	return repo, cleanup
}

func createUnseededPromptDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
	})
	if _, err := sqlxDB.Exec(`
		CREATE TABLE custom_prompts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return sqlxDB
}

func seedStoredWalkthroughPrompt(t *testing.T, sqlxDB *sqlx.DB, content string, createdAt, updatedAt time.Time) {
	t.Helper()
	if _, err := sqlxDB.Exec(
		`INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
		"builtin-changes-walkthrough", "changes-walkthrough", content, createdAt, updatedAt,
	); err != nil {
		t.Fatalf("seed stored walkthrough prompt: %v", err)
	}
}

func readPromptFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read prompt fixture: %v", err)
	}
	return string(content)
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
	// Should have 1 custom prompt + built-in prompts.
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
	// Should only have built-in prompts left.
	builtinCount := 0
	for _, p := range list {
		if p.Builtin {
			builtinCount++
		}
		if p.ID == prompt.ID {
			t.Fatalf("expected custom prompt to be deleted, but it still exists")
		}
	}
	if builtinCount != 5 {
		t.Fatalf("expected 5 built-in prompts, got %d", builtinCount)
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

	// Should include the CI auto-fix and changes walkthrough built-in prompts.
	builtinCount := 0
	var ciAutoFixContent string
	var changesWalkthroughContent string
	for _, p := range list {
		if p.Builtin {
			builtinCount++
		}
		if p.ID == "builtin-ci-auto-fix" && p.Name == "ci-auto-fix" && p.Content != "" {
			ciAutoFixContent = p.Content
		}
		if p.ID == "builtin-changes-walkthrough" && p.Name == "changes-walkthrough" && p.Content != "" {
			changesWalkthroughContent = p.Content
		}
	}

	if builtinCount != 5 {
		t.Fatalf("expected 5 built-in prompts, got %d", builtinCount)
	}
	if ciAutoFixContent == "" {
		t.Fatalf("expected ci-auto-fix built-in prompt")
	}
	for _, want := range []string{
		"If the new feedback is not actionable",
		"do not modify files",
		"do not commit",
		"do not push",
		"nothing actionable to address",
		"{{pr.feedback}}",
		"resolve the addressed PR review threads",
	} {
		if !strings.Contains(ciAutoFixContent, want) {
			t.Fatalf("expected ci-auto-fix prompt to contain %q, got:\n%s", want, ciAutoFixContent)
		}
	}
	if changesWalkthroughContent == "" {
		t.Fatalf("expected changes-walkthrough built-in prompt")
	}
	for _, want := range []string{
		"show_walkthrough_kandev",
		"Inspect the changed files yourself",
		"compare the PR head against the PR base branch",
		"compare against the task/repository base",
		"Use `line_end`",
		"do not assume the PR head is checked out locally",
		"first walkthrough step",
		"ELI5:",
	} {
		if !strings.Contains(changesWalkthroughContent, want) {
			t.Fatalf("expected changes-walkthrough prompt to contain %q, got:\n%s", want, changesWalkthroughContent)
		}
	}
}

func TestSQLiteRepository_BuiltinSeedIgnoresUserPromptNameConflict(t *testing.T) {
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	defer func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
	}()
	if _, err := sqlxDB.Exec(`
		CREATE TABLE custom_prompts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := sqlxDB.Exec(
		`INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"user-walkthrough", "changes-walkthrough", "user-owned prompt", now, now,
	); err != nil {
		t.Fatalf("seed user prompt: %v", err)
	}

	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("repo init should ignore built-in name conflicts: %v", err)
	}
	got, err := repo.GetPromptByName(context.Background(), "changes-walkthrough")
	if err != nil {
		t.Fatalf("get existing prompt: %v", err)
	}
	if got.ID != "user-walkthrough" || got.Builtin {
		t.Fatalf("expected user prompt to remain canonical, got %+v", got)
	}
}

func TestSQLiteRepository_RefreshesUnmodifiedLegacyChangesWalkthroughPrompt(t *testing.T) {
	for _, fixture := range []string{
		"changes-walkthrough-v1.md",
		"changes-walkthrough-v2.md",
	} {
		t.Run(fixture, func(t *testing.T) {
			sqlxDB := createUnseededPromptDB(t)
			legacyContent := strings.TrimSpace(readPromptFixture(t, fixture))
			now := time.Now().UTC()
			seedStoredWalkthroughPrompt(t, sqlxDB, legacyContent, now, now)

			repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
			if err != nil {
				t.Fatalf("initialize repository: %v", err)
			}
			got, err := repo.GetPromptByName(context.Background(), "changes-walkthrough")
			if err != nil {
				t.Fatalf("get refreshed prompt: %v", err)
			}
			if got.Content != promptcfg.Get("changes-walkthrough") {
				t.Fatalf("stored legacy prompt was not refreshed")
			}
		})
	}
}

func TestSQLiteRepository_PreservesEditedLegacyChangesWalkthroughPrompt(t *testing.T) {
	sqlxDB := createUnseededPromptDB(t)
	legacyContent := readPromptFixture(t, "changes-walkthrough-v2.md")
	createdAt := time.Now().UTC().Add(-time.Hour)
	seedStoredWalkthroughPrompt(t, sqlxDB, legacyContent, createdAt, createdAt.Add(time.Minute))

	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	got, err := repo.GetPromptByName(context.Background(), "changes-walkthrough")
	if err != nil {
		t.Fatalf("get edited prompt: %v", err)
	}
	if got.Content != legacyContent {
		t.Fatalf("user-edited built-in prompt was overwritten")
	}
}

func TestSQLiteRepository_PreservesUntouchedUnrecognizedChangesWalkthroughPrompt(t *testing.T) {
	sqlxDB := createUnseededPromptDB(t)
	customContent := "this is not a recognized legacy revision"
	now := time.Now().UTC()
	seedStoredWalkthroughPrompt(t, sqlxDB, customContent, now, now)

	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	got, err := repo.GetPromptByName(context.Background(), "changes-walkthrough")
	if err != nil {
		t.Fatalf("get unrecognized prompt: %v", err)
	}
	if got.Content != customContent {
		t.Fatalf("unrecognized untouched content was overwritten")
	}
}
