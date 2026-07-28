package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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

func TestDeleteRepositoryIfUnreferenced_PreservesTaskAdoptedRepository(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cleanup-reference")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-cleanup-reference", WorkspaceID: "ws-cleanup-reference", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-cleanup-reference", WorkspaceID: "ws-cleanup-reference", Name: "app"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-cleanup-reference", WorkspaceID: "ws-cleanup-reference", WorkflowID: "wf-cleanup-reference", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-cleanup-reference", TaskID: "task-cleanup-reference", RepositoryID: "repo-cleanup-reference", BaseBranch: "main"}); err != nil {
		t.Fatal(err)
	}

	deleted, err := repo.DeleteRepositoryIfUnreferenced(ctx, "repo-cleanup-reference")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("cleanup deleted repository adopted by a task")
	}
	if _, err := repo.GetRepository(ctx, "repo-cleanup-reference"); err != nil {
		t.Fatalf("repository after rejected cleanup: %v", err)
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

func TestRepositoryProviderHost_RoundTrip(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-provider-host")
	in := &models.Repository{
		ID: "repo-gitlab", WorkspaceID: "ws-provider-host", Name: "group/subgroup/project",
		SourceType: "provider", Provider: "gitlab", ProviderHost: "http://gitlab.internal:8080",
		ProviderOwner: "group/subgroup", ProviderName: "project",
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.ProviderHost != in.ProviderHost {
		t.Fatalf("provider_host = %q, want %q", got.ProviderHost, in.ProviderHost)
	}
	got.ProviderHost = "https://gitlab.internal"
	if err := repo.UpdateRepository(ctx, got); err != nil {
		t.Fatalf("update repository: %v", err)
	}
	updated, err := repo.GetRepository(ctx, in.ID)
	if err != nil || updated.ProviderHost != "https://gitlab.internal" {
		t.Fatalf("updated provider_host = %q, err = %v", updated.ProviderHost, err)
	}
}

func TestGetRepositoryByProviderInfoSeparatesGitLabHosts(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-host-collision")
	for _, item := range []*models.Repository{
		{ID: "repo-public", WorkspaceID: "ws-host-collision", Name: "public", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.com", ProviderOwner: "group/subgroup", ProviderName: "project"},
		{ID: "repo-private", WorkspaceID: "ws-host-collision", Name: "private", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.internal", ProviderOwner: "group/subgroup", ProviderName: "project"},
	} {
		if err := repo.CreateRepository(ctx, item); err != nil {
			t.Fatalf("create repository %s: %v", item.ID, err)
		}
	}
	got, err := repo.GetRepositoryByProviderInfo(
		ctx, "ws-host-collision", "gitlab", "https://gitlab.internal", "group/subgroup", "project",
	)
	if err != nil || got == nil || got.ID != "repo-private" {
		t.Fatalf("host-aware lookup = %+v, err = %v; want repo-private", got, err)
	}
}

// TestGetRepositoryByProviderInfoReturnsEarliestCreatedDuplicate guards the
// Greptile-flagged race window: when two rows already share the same
// provider identity (left over from a resolver race that predates
// Service.repoResolveMu), GetRepositoryByProviderInfo must resolve to the
// same row ListRepositories' dedupeRepositoriesByIdentity keeps as the
// canonical winner (earliest created_at, ties broken by the smaller id) —
// not an arbitrary one of the two — otherwise a caller can attach a task to,
// or backfill fields onto, the duplicate ListRepositories hides.
func TestGetRepositoryByProviderInfoReturnsEarliestCreatedDuplicate(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-provider-dup")
	for _, item := range []*models.Repository{
		{ID: "repo-dup-later", WorkspaceID: "ws-provider-dup", Name: "later", SourceType: "provider", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "kdlbs", ProviderName: "kandev"},
		{ID: "repo-dup-earlier", WorkspaceID: "ws-provider-dup", Name: "earlier", SourceType: "provider", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "kdlbs", ProviderName: "kandev"},
	} {
		if err := repo.CreateRepository(ctx, item); err != nil {
			t.Fatalf("create repository %s: %v", item.ID, err)
		}
	}
	// CreateRepository always stamps created_at = time.Now(), so backdate the
	// intended winner directly to make ordering deterministic regardless of
	// wall-clock resolution.
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE repositories SET created_at = ? WHERE id = ?`), earlier, "repo-dup-earlier"); err != nil {
		t.Fatalf("backdate repo-dup-earlier: %v", err)
	}

	got, err := repo.GetRepositoryByProviderInfo(ctx, "ws-provider-dup", "github", "https://github.com", "kdlbs", "kandev")
	if err != nil {
		t.Fatalf("GetRepositoryByProviderInfo: %v", err)
	}
	if got == nil || got.ID != "repo-dup-earlier" {
		t.Fatalf("GetRepositoryByProviderInfo = %+v, want repo-dup-earlier (the row ListRepositories keeps as canonical)", got)
	}
}

func TestRepositoryProviderHostMigrationBackfillsOnlyUnambiguousGitHubRows(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-provider-upgrade")
	for _, item := range []*models.Repository{
		{ID: "legacy-github", WorkspaceID: "ws-provider-upgrade", Name: "org/repo", SourceType: "provider", Provider: "github", ProviderOwner: "org", ProviderName: "repo"},
		{ID: "legacy-gitlab", WorkspaceID: "ws-provider-upgrade", Name: "group/repo", SourceType: "provider", Provider: "gitlab", ProviderOwner: "group", ProviderName: "repo"},
	} {
		if err := repo.CreateRepository(ctx, item); err != nil {
			t.Fatalf("create legacy repository %s: %v", item.ID, err)
		}
	}

	if err := repo.initSchema(); err != nil {
		t.Fatalf("first upgrade replay: %v", err)
	}
	if err := repo.initSchema(); err != nil {
		t.Fatalf("second upgrade replay: %v", err)
	}

	githubRepo, err := repo.GetRepository(ctx, "legacy-github")
	if err != nil || githubRepo.ProviderHost != "https://github.com" {
		t.Fatalf("GitHub provider_host = %q, err = %v", githubRepo.ProviderHost, err)
	}
	gitlabRepo, err := repo.GetRepository(ctx, "legacy-gitlab")
	if err != nil || gitlabRepo.ProviderHost != "" {
		t.Fatalf("GitLab provider_host = %q, err = %v; want unknown", gitlabRepo.ProviderHost, err)
	}
}

func TestGetRepositoryReturnsNotFoundError(t *testing.T) {
	repo := newRepoForEntityTests(t)
	_, err := repo.GetRepository(context.Background(), "missing")
	if !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("GetRepository error = %v, want ErrRepositoryNotFound", err)
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
