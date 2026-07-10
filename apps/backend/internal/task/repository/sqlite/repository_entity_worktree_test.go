package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newRepoForRepositoryTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "repo-test.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func seedWorkspace(t *testing.T, repo *Repository, id string) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO workspaces (id, name, created_at, updated_at)
		VALUES (?, 'ws', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`), id)
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
}

func TestRepositoryEntity_WorktreeFilesRoundtrip(t *testing.T) {
	repo := newRepoForRepositoryTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-1")

	in := &models.Repository{
		WorkspaceID: "ws-1",
		Name:        "with-files",
		SourceType:  "local",
		WorktreeFiles: []models.WorktreeFile{
			{Path: ".env.local", Mode: "copy"},
			{Path: "config/secrets.env", Mode: "symlink"},
		},
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if len(got.WorktreeFiles) != 2 {
		t.Fatalf("WorktreeFiles = %v, want 2 entries", got.WorktreeFiles)
	}
	if got.WorktreeFiles[0] != (models.WorktreeFile{Path: ".env.local", Mode: "copy"}) {
		t.Errorf("file[0] = %+v", got.WorktreeFiles[0])
	}
	if got.WorktreeFiles[1] != (models.WorktreeFile{Path: "config/secrets.env", Mode: "symlink"}) {
		t.Errorf("file[1] = %+v", got.WorktreeFiles[1])
	}

	// Update the list.
	got.WorktreeFiles = []models.WorktreeFile{{Path: ".env", Mode: "symlink"}}
	if err := repo.UpdateRepository(ctx, got); err != nil {
		t.Fatalf("UpdateRepository: %v", err)
	}
	after, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetRepository after update: %v", err)
	}
	if len(after.WorktreeFiles) != 1 || after.WorktreeFiles[0] != (models.WorktreeFile{Path: ".env", Mode: "symlink"}) {
		t.Errorf("after update: files=%+v", after.WorktreeFiles)
	}
}

// TestRepositoryEntity_GetByLocalPath verifies repositories (and their worktree
// file config) can be resolved by local_path — the lookup used during worktree
// creation, where the RepositoryID isn't available.
func TestRepositoryEntity_GetByLocalPath(t *testing.T) {
	repo := newRepoForRepositoryTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-1")

	in := &models.Repository{
		WorkspaceID:   "ws-1",
		Name:          "cairn",
		SourceType:    "local",
		LocalPath:     "/home/dev/playground/cairn",
		WorktreeFiles: []models.WorktreeFile{{Path: ".env", Mode: "symlink"}},
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	got, err := repo.GetRepositoryByLocalPath(ctx, "/home/dev/playground/cairn")
	if err != nil {
		t.Fatalf("GetRepositoryByLocalPath: %v", err)
	}
	if got == nil || got.ID != in.ID {
		t.Fatalf("expected to resolve repo by path, got %+v", got)
	}
	if len(got.WorktreeFiles) != 1 || got.WorktreeFiles[0].Mode != "symlink" {
		t.Fatalf("worktree files not loaded via path lookup: %+v", got.WorktreeFiles)
	}

	// Unknown path returns (nil, nil).
	missing, err := repo.GetRepositoryByLocalPath(ctx, "/nope")
	if err != nil || missing != nil {
		t.Fatalf("expected (nil,nil) for unknown path, got %+v, %v", missing, err)
	}
}

// TestRepositoryEntity_DefaultsBackwardCompat verifies a repository created
// without any worktree-file config loads with an empty list, mirroring rows
// written before the feature existed.
func TestRepositoryEntity_DefaultsBackwardCompat(t *testing.T) {
	repo := newRepoForRepositoryTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-1")

	in := &models.Repository{WorkspaceID: "ws-1", Name: "legacy", SourceType: "local"}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if len(got.WorktreeFiles) != 0 {
		t.Errorf("default files = %v, want empty", got.WorktreeFiles)
	}
}
