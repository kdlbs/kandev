package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newRepoForCredentialOriginMigrationTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "repository-credential-origin-migration.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func readRepositoryProviderHost(t *testing.T, repo *Repository, id string) string {
	t.Helper()
	var host string
	if err := repo.db.QueryRow(repo.db.Rebind(
		`SELECT provider_host FROM repositories WHERE id = ?`), id,
	).Scan(&host); err != nil {
		t.Fatalf("read provider_host for %s: %v", id, err)
	}
	return host
}

// TestRepositoryCredentialOriginBackfillFillsLegacyAndPluginGitHubRows covers
// AC-10: a repository whose provider column is empty or a plugin identifier,
// but whose persisted remote is an exact public GitHub clone URL (any
// supported spelling), gets provider_host backfilled so managed credential
// issuance stops failing closed post-f218880ec.
func TestRepositoryCredentialOriginBackfillFillsLegacyAndPluginGitHubRows(t *testing.T) {
	repo := newRepoForCredentialOriginMigrationTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cred-origin")

	rows := []*models.Repository{
		{ID: "repo-empty-provider-ssh", WorkspaceID: "ws-cred-origin", Name: "a", SourceType: "local",
			RemoteURL: "git@github.com:acme/widgets.git"},
		{ID: "repo-empty-provider-https", WorkspaceID: "ws-cred-origin", Name: "b", SourceType: "local",
			RemoteURL: "https://github.com/acme/widgets.git"},
		{ID: "repo-plugin-provider", WorkspaceID: "ws-cred-origin", Name: "c", SourceType: "local",
			Provider: "kandev-plugin-tags", RemoteURL: "ssh://git@github.com/acme/widgets.git"},
	}
	for _, repository := range rows {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("create repository %s: %v", repository.ID, err)
		}
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	for _, repository := range rows {
		if got := readRepositoryProviderHost(t, repo, repository.ID); got != "https://github.com" {
			t.Errorf("provider_host for %s = %q, want https://github.com", repository.ID, got)
		}
	}
}

// TestRepositoryCredentialOriginBackfillSkipsAmbiguousAndUnsafeRows covers
// AC-4/AC-10: a custom host with no trustworthy provider host metadata, a
// credential-bearing remote, and an unparsable remote must all be left alone
// rather than guessed at.
func TestRepositoryCredentialOriginBackfillSkipsAmbiguousAndUnsafeRows(t *testing.T) {
	repo := newRepoForCredentialOriginMigrationTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cred-origin-skip")

	rows := []*models.Repository{
		{ID: "repo-custom-host", WorkspaceID: "ws-cred-origin-skip", Name: "custom", SourceType: "local",
			RemoteURL: "git@gitlab.example.com:acme/widgets.git"},
		{ID: "repo-credential-bearing", WorkspaceID: "ws-cred-origin-skip", Name: "creds", SourceType: "local",
			RemoteURL: "https://user:pass@github.com/acme/widgets.git"},
		{ID: "repo-no-remote", WorkspaceID: "ws-cred-origin-skip", Name: "none", SourceType: "local"},
		{ID: "repo-malformed", WorkspaceID: "ws-cred-origin-skip", Name: "malformed", SourceType: "local",
			RemoteURL: "not-a-valid-url"},
	}
	for _, repository := range rows {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("create repository %s: %v", repository.ID, err)
		}
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	for _, repository := range rows {
		if got := readRepositoryProviderHost(t, repo, repository.ID); got != "" {
			t.Errorf("provider_host for %s = %q, want left empty", repository.ID, got)
		}
	}
}

// TestRepositoryCredentialOriginBackfillPreservesIdentityAndTaskBindings
// covers AC-10's "preserves id, provider, remote_url, task bindings" clause:
// only provider_host may change, and an existing task -> repository link
// survives untouched.
func TestRepositoryCredentialOriginBackfillPreservesIdentityAndTaskBindings(t *testing.T) {
	repo := newRepoForCredentialOriginMigrationTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cred-origin-preserve")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-cred-origin", WorkspaceID: "ws-cred-origin-preserve", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	repository := &models.Repository{
		ID: "repo-preserve", WorkspaceID: "ws-cred-origin-preserve", Name: "preserve", SourceType: "local",
		Provider: "kandev-plugin-tags", RemoteURL: "git@github.com:acme/widgets.git",
	}
	if err := repo.CreateRepository(ctx, repository); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-preserve", WorkspaceID: "ws-cred-origin-preserve", WorkflowID: "wf-cred-origin", Title: "Task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-preserve", TaskID: "task-preserve", RepositoryID: repository.ID,
	}); err != nil {
		t.Fatalf("create task repository link: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	updated, err := repo.GetRepository(ctx, repository.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if updated.ID != repository.ID || updated.Provider != repository.Provider || updated.RemoteURL != repository.RemoteURL {
		t.Fatalf("repository identity changed: got %+v", updated)
	}
	if updated.ProviderHost != "https://github.com" {
		t.Fatalf("provider_host = %q, want https://github.com", updated.ProviderHost)
	}

	links, err := repo.ListTaskRepositories(ctx, "task-preserve")
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	if len(links) != 1 || links[0].RepositoryID != repository.ID {
		t.Fatalf("task repository bindings changed: got %+v", links)
	}
}

// TestRepositoryCredentialOriginBackfillLeavesExistingProviderHostAlone
// covers AC-10's "preserve custom/ambiguous rows" clause the other direction:
// a row that already carries a provider_host, even a GitHub Enterprise host
// unrelated to public GitHub, is never touched by this backfill.
func TestRepositoryCredentialOriginBackfillLeavesExistingProviderHostAlone(t *testing.T) {
	repo := newRepoForCredentialOriginMigrationTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cred-origin-existing")

	repository := &models.Repository{
		ID: "repo-existing-host", WorkspaceID: "ws-cred-origin-existing", Name: "existing", SourceType: "local",
		ProviderHost: "https://ghe.example", RemoteURL: "git@ghe.example:acme/widgets.git",
	}
	if err := repo.CreateRepository(ctx, repository); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	if got := readRepositoryProviderHost(t, repo, repository.ID); got != "https://ghe.example" {
		t.Fatalf("provider_host = %q, want unchanged https://ghe.example", got)
	}
}

// TestRepositoryCredentialOriginBackfillIsReplaySafe covers AC-10's replay
// requirement: a second runMigrations() call over already-backfilled and
// already-skipped rows changes nothing and returns no error.
func TestRepositoryCredentialOriginBackfillIsReplaySafe(t *testing.T) {
	repo := newRepoForCredentialOriginMigrationTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cred-origin-replay")

	backfillable := &models.Repository{
		ID: "repo-replay-backfillable", WorkspaceID: "ws-cred-origin-replay", Name: "a", SourceType: "local",
		RemoteURL: "git@github.com:acme/widgets.git",
	}
	skipped := &models.Repository{
		ID: "repo-replay-skipped", WorkspaceID: "ws-cred-origin-replay", Name: "b", SourceType: "local",
		RemoteURL: "git@gitlab.example.com:acme/widgets.git",
	}
	for _, repository := range []*models.Repository{backfillable, skipped} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("create repository %s: %v", repository.ID, err)
		}
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}

	if got := readRepositoryProviderHost(t, repo, backfillable.ID); got != "https://github.com" {
		t.Fatalf("provider_host for backfillable row = %q, want https://github.com", got)
	}
	if got := readRepositoryProviderHost(t, repo, skipped.ID); got != "" {
		t.Fatalf("provider_host for skipped row = %q, want left empty", got)
	}
}
