package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

// newSearchTestRepo creates a test repo with a minimal tasks table for search tests.
func newSearchTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			assignee_agent_instance_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return repo
}

func insertTask(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, wsID, title, desc, identifier string) {
	t.Helper()
	_, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, description, identifier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, desc, identifier)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func TestSearchTasks_MatchesTitle(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Fix login bug", "desc here", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Add payment flow", "payment desc", "KAN-2")
	insertTask(t, repo, ctx, "t3", "ws1", "Refactor auth", "auth refactor", "KAN-3")

	results, err := repo.SearchTasks(ctx, "ws1", "login", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Fix login bug" {
		t.Errorf("expected title 'Fix login bug', got %q", results[0].Title)
	}
}

func TestSearchTasks_MatchesIdentifier(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Some task", "some desc", "KAN-42")
	insertTask(t, repo, ctx, "t2", "ws1", "Another task", "another desc", "KAN-99")

	results, err := repo.SearchTasks(ctx, "ws1", "KAN-42", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Identifier != "KAN-42" {
		t.Errorf("expected identifier 'KAN-42', got %q", results[0].Identifier)
	}
}

func TestSearchTasks_MatchesDescription(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Title A", "fix the authentication module", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Title B", "update the readme", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "authentication", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task t1, got %q", results[0].ID)
	}
}

func TestSearchTasks_NoResults(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Some task", "desc", "KAN-1")

	results, err := repo.SearchTasks(ctx, "ws1", "nonexistent", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchTasks_RespectsLimit(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("t%d", i)
		ident := fmt.Sprintf("KAN-%d", i)
		insertTask(t, repo, ctx, id, "ws1", "Match task", "desc", ident)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "Match", 3)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

func TestSearchTasks_WorkspaceIsolation(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Shared title", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws2", "Shared title", "desc", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "Shared", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ws1, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task from ws1, got %q", results[0].ID)
	}
}

func TestSearchTasks_ExcludesArchived(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Active task", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Archived task", "desc", "KAN-2")
	// Archive t2
	_, err := repo.ExecRaw(ctx, `UPDATE tasks SET archived_at = datetime('now') WHERE id = ?`, "t2")
	if err != nil {
		t.Fatalf("archive task: %v", err)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "task", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (archived excluded), got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected active task, got %q", results[0].ID)
	}
}

func TestSearchTasks_ExcludesEphemeral(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Normal task", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Ephemeral task", "desc", "KAN-2")
	// Mark t2 as ephemeral
	_, err := repo.ExecRaw(ctx, `UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`, "t2")
	if err != nil {
		t.Fatalf("set ephemeral: %v", err)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "task", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (ephemeral excluded), got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected normal task, got %q", results[0].ID)
	}
}
