package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

func createTestSQLiteRepo(t *testing.T) (*sqlite.Repository, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("failed to create SQLite repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
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

func TestNewSQLiteRepositoryWithDB(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
	if repo.DB() == nil {
		t.Error("expected db to be initialized")
	}
}

func TestSQLiteRepository_Close(t *testing.T) {
	repo, _ := createTestSQLiteRepo(t)
	err := repo.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}
}

func TestSQLiteRepository_SeedsDefaultWorkspace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workspaces, err := repo.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("failed to list workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}
	if workspaces[0].Name != "Default Workspace" {
		t.Errorf("expected Default Workspace, got %s", workspaces[0].Name)
	}

	workflows, err := repo.ListWorkflows(ctx, workspaces[0].ID)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
	if workflows[0].Name != "Development" {
		t.Errorf("expected Development workflow, got %s", workflows[0].Name)
	}
	// Note: workflow steps are now managed by the workflow repository, not the task repository
}
