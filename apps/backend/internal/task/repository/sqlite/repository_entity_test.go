package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newRepoForEntityTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "repo-entity-test.db")
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
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id}); err != nil {
		t.Fatalf("seed workspace %s: %v", id, err)
	}
}

// TestRepositoryCopyFiles_RoundTrip writes a repository with a non-empty
// CopyFiles, fetches it back via GetRepository and ListRepositories, and
// asserts the value survived both code paths.
func TestRepositoryCopyFiles_RoundTrip(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-copy")

	in := &models.Repository{
		ID:          "repo-copy-1",
		WorkspaceID: "ws-copy",
		Name:        "with-copy-files",
		SourceType:  "local",
		CopyFiles:   ".env, *.local",
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.CopyFiles != ".env, *.local" {
		t.Errorf("GetRepository CopyFiles = %q, want %q", got.CopyFiles, ".env, *.local")
	}

	list, err := repo.ListRepositories(ctx, "ws-copy")
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(list) != 1 || list[0].CopyFiles != ".env, *.local" {
		t.Errorf("ListRepositories CopyFiles = %v, want one repo with %q", list, ".env, *.local")
	}
}

// TestRepositoryCopyFiles_Update creates a repo with an empty CopyFiles
// value, mutates the model in-memory, calls UpdateRepository, and verifies
// the new value is persisted.
func TestRepositoryCopyFiles_Update(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-copy-upd")

	in := &models.Repository{
		ID:          "repo-copy-upd",
		WorkspaceID: "ws-copy-upd",
		Name:        "update-target",
		SourceType:  "local",
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	in.CopyFiles = ".env"
	if err := repo.UpdateRepository(ctx, in); err != nil {
		t.Fatalf("update repository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.CopyFiles != ".env" {
		t.Errorf("after update, CopyFiles = %q, want %q", got.CopyFiles, ".env")
	}
}

// TestRepositoryCopyFiles_DefaultEmpty ensures older callers that don't
// populate CopyFiles round-trip to an empty string rather than panicking on
// a NULL scan.
func TestRepositoryCopyFiles_DefaultEmpty(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-copy-def")

	in := &models.Repository{
		ID:          "repo-copy-def",
		WorkspaceID: "ws-copy-def",
		Name:        "no-copy-files",
		SourceType:  "local",
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.CopyFiles != "" {
		t.Errorf("default CopyFiles = %q, want empty string", got.CopyFiles)
	}
}

func TestDeleteRepositoryIfNoActiveTaskSessions(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		state   string
		deleted bool
	}{
		{name: "completed session", state: "COMPLETED", deleted: true},
		{name: "idle session", state: "IDLE", deleted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			seedRepoLink(t, repo, "ws-1", "repo-1", "task-1", "session-1", tc.state)

			deleted, err := repo.DeleteRepositoryIfNoActiveTaskSessions(ctx, "repo-1")
			if err != nil {
				t.Fatalf("DeleteRepositoryIfNoActiveTaskSessions: %v", err)
			}
			if deleted != tc.deleted {
				t.Fatalf("deleted = %v, want %v", deleted, tc.deleted)
			}
			_, err = repo.GetRepository(ctx, "repo-1")
			if tc.deleted && err == nil {
				t.Fatal("deleted repository remains live")
			}
			if !tc.deleted && err != nil {
				t.Fatalf("retained repository was deleted: %v", err)
			}
		})
	}
}

// TestRunMigrations_Idempotent verifies that re-running migrations on an
// already-migrated schema does not error (Apply swallows "duplicate column"
// failures by design).
func TestRunMigrations_Idempotent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("second runMigrations call returned error: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("third runMigrations call returned error: %v", err)
	}
}
