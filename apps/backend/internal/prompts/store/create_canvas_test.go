package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCreateCanvasBuiltin(t *testing.T) {
	t.Run("seeds the complete authoring workflow", func(t *testing.T) {
		repo, cleanup := createTestRepo(t)
		defer cleanup()

		prompt, err := repo.GetPromptByName(context.Background(), "create-canvas")
		if err != nil {
			t.Fatalf("get create-canvas prompt: %v", err)
		}
		if prompt.ID != "builtin-create-canvas" || !prompt.Builtin {
			t.Fatalf("unexpected prompt identity: %+v", prompt)
		}
		for _, want := range []string{
			"create_canvas_kandev",
			"read_canvas_authoring_skill_kandev",
			"publish_canvas_kandev",
			"native tool search",
			"source directory",
			"authorized live Kandev data",
			"awaits permission review",
			"requires promotion",
			"local build alone does not publish",
		} {
			if !strings.Contains(prompt.Content, want) {
				t.Fatalf("create-canvas prompt is missing %q", want)
			}
		}
	})

	t.Run("preserves a user prompt with the same name", func(t *testing.T) {
		db := createUnseededPromptDB(t)
		now := time.Now().UTC()
		if _, err := db.Exec(
			`INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
			"user-create-canvas", "create-canvas", "user-owned prompt", now, now,
		); err != nil {
			t.Fatalf("seed user prompt: %v", err)
		}

		repo, err := newSQLiteRepositoryWithDB(db, db)
		if err != nil {
			t.Fatalf("initialize repository: %v", err)
		}
		got, err := repo.GetPromptByName(context.Background(), "create-canvas")
		if err != nil {
			t.Fatalf("get user prompt: %v", err)
		}
		if got.ID != "user-create-canvas" || got.Builtin || got.Content != "user-owned prompt" {
			t.Fatalf("user prompt was replaced: %+v", got)
		}
	})

	t.Run("preserves user edits when the database is reopened", func(t *testing.T) {
		db := createUnseededPromptDB(t)
		repo, err := newSQLiteRepositoryWithDB(db, db)
		if err != nil {
			t.Fatalf("initialize repository: %v", err)
		}
		prompt, err := repo.GetPromptByName(context.Background(), "create-canvas")
		if err != nil {
			t.Fatalf("get seeded prompt: %v", err)
		}
		prompt.Content = "user-edited canvas workflow"
		if err := repo.UpdatePrompt(context.Background(), prompt); err != nil {
			t.Fatalf("update prompt: %v", err)
		}

		if _, err := newSQLiteRepositoryWithDB(db, db); err != nil {
			t.Fatalf("reopen repository: %v", err)
		}
		got, err := repo.GetPromptByName(context.Background(), "create-canvas")
		if err != nil {
			t.Fatalf("get edited prompt: %v", err)
		}
		if got.Content != "user-edited canvas workflow" {
			t.Fatalf("user edit was replaced with %q", got.Content)
		}
	})
}
