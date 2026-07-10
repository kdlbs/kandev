package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

// TestRepositoryStartupPrompt_RoundTrip verifies the startup_prompt column
// survives create / update / get / list; guards against regressions in the
// SQL projection lists across those queries.
func TestRepositoryStartupPrompt_RoundTrip(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sp")

	prompt := "Read {{TICKET_URL}} and start work.\nAcceptance criteria are in the ticket."
	in := &models.Repository{
		ID:            "repo-sp-1",
		WorkspaceID:   "ws-sp",
		Name:          "with-startup-prompt",
		SourceType:    "local",
		StartupPrompt: prompt,
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.StartupPrompt != prompt {
		t.Errorf("after create, StartupPrompt = %q, want %q", got.StartupPrompt, prompt)
	}

	got.StartupPrompt = "Only {{TASK_TITLE}} left."
	if err := repo.UpdateRepository(ctx, got); err != nil {
		t.Fatalf("update repository: %v", err)
	}
	afterUpdate, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if afterUpdate.StartupPrompt != "Only {{TASK_TITLE}} left." {
		t.Errorf("after update, StartupPrompt = %q", afterUpdate.StartupPrompt)
	}

	list, err := repo.ListRepositories(ctx, "ws-sp")
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(list) != 1 || list[0].StartupPrompt != "Only {{TASK_TITLE}} left." {
		t.Errorf("ListRepositories StartupPrompt mismatch: %+v", list)
	}
}

// TestRepositoryStartupPrompt_LegacyRowScansAsEmpty simulates a row inserted
// by a caller that pre-dates the startup_prompt column — i.e. an INSERT that
// never mentions the column and therefore falls back to the schema DEFAULT ''.
// Guards against a nil-scan panic in GetRepository / ListRepositories on such
// rows and pins the DEFAULT '' contract.
func TestRepositoryStartupPrompt_LegacyRowScansAsEmpty(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sp-legacy")

	// Bypass CreateRepository so no code path writes startup_prompt explicitly;
	// the value comes from the column DEFAULT.
	now := time.Now().UTC()
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO repositories (
			id, workspace_id, name, source_type, local_path, provider,
			provider_repo_id, provider_owner, provider_name, default_branch,
			worktree_branch_prefix, pull_before_worktree, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "legacy-repo", "ws-sp-legacy", "legacy", "local", "", "", "", "", "", "", "feature/", 1, now, now)
	if err != nil {
		t.Fatalf("legacy insert: %v", err)
	}

	got, err := repo.GetRepository(ctx, "legacy-repo")
	if err != nil {
		t.Fatalf("get legacy repository: %v", err)
	}
	if got.StartupPrompt != "" {
		t.Errorf("legacy StartupPrompt = %q, want empty", got.StartupPrompt)
	}

	list, err := repo.ListRepositories(ctx, "ws-sp-legacy")
	if err != nil {
		t.Fatalf("list legacy repositories: %v", err)
	}
	if len(list) != 1 || list[0].StartupPrompt != "" {
		t.Errorf("ListRepositories StartupPrompt on legacy row: %+v", list)
	}
}

// TestRepositoryStartupPrompt_NotNullRejectsNullInsert pins the schema
// contract that startup_prompt cannot be NULL — no code path (nor a
// hand-written SQL client) can leave a row that would fail the Go string
// scan in GetRepository / ListRepositories.
func TestRepositoryStartupPrompt_NotNullRejectsNullInsert(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sp-null")

	now := time.Now().UTC()
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO repositories (
			id, workspace_id, name, source_type, startup_prompt,
			worktree_branch_prefix, pull_before_worktree, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?)
	`), "null-repo", "ws-sp-null", "null", "local", "feature/", 1, now, now)
	if err == nil {
		t.Fatal("expected NOT NULL constraint violation on startup_prompt = NULL insert")
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
