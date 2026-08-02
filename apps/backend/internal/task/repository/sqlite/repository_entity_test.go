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

func strptr(value string) *string { return &value }

func TestCreateWorkflowRejectsDuplicateHiddenTemplatePerWorkspace(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-template-unique")

	first := &models.Workflow{
		ID:                 "wf-template-first",
		WorkspaceID:        "ws-template-unique",
		Name:               "Improve Kandev",
		WorkflowTemplateID: strptr("improve-kandev"),
		Hidden:             true,
	}
	if err := repo.CreateWorkflow(ctx, first); err != nil {
		t.Fatalf("create first template workflow: %v", err)
	}
	duplicate := &models.Workflow{
		ID:                 "wf-template-duplicate",
		WorkspaceID:        first.WorkspaceID,
		Name:               first.Name,
		WorkflowTemplateID: strptr("improve-kandev"),
		Hidden:             true,
	}
	if err := repo.CreateWorkflow(ctx, duplicate); err == nil {
		t.Fatal("duplicate hidden template workflow was accepted")
	}
	for _, id := range []string{"wf-other-template-first", "wf-other-template-second"} {
		if err := repo.CreateWorkflow(ctx, &models.Workflow{
			ID:                 id,
			WorkspaceID:        first.WorkspaceID,
			Name:               "Reusable template workflow",
			WorkflowTemplateID: strptr("reusable-template"),
			Hidden:             true,
		}); err != nil {
			t.Fatalf("create non-Improve-Kandev template workflow %q: %v", id, err)
		}
	}
}

func TestImproveKandevWorkflowIndexMigratesFormerBroadIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "improve-kandev-index-replay.db")
	openRepo := func() (*Repository, *sqlx.DB) {
		t.Helper()
		dbConn, err := db.OpenSQLite(dbPath)
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
		repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
		if err != nil {
			_ = sqlxDB.Close()
			t.Fatalf("new repo: %v", err)
		}
		return repo, sqlxDB
	}

	repo, sqlxDB := openRepo()
	seedWorkspace(t, repo, "ws-improve-kandev-index-replay")
	if _, err := sqlxDB.Exec(`DROP INDEX IF EXISTS uniq_improve_kandev_workflows`); err != nil {
		t.Fatalf("drop scoped index: %v", err)
	}
	if _, err := sqlxDB.Exec(`DROP INDEX IF EXISTS uniq_workflows_workspace_template_hidden`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if _, err := sqlxDB.Exec(`CREATE UNIQUE INDEX uniq_workflows_workspace_template_hidden
		ON workflows(workspace_id, workflow_template_id, hidden)
		WHERE workflow_template_id <> ''`); err != nil {
		t.Fatalf("create former broad index: %v", err)
	}
	if err := sqlxDB.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	repo, sqlxDB = openRepo()
	t.Cleanup(func() { _ = sqlxDB.Close() })
	for indexName, want := range map[string]int{
		"uniq_workflows_workspace_template_hidden": 0,
		"uniq_improve_kandev_workflows":            1,
	} {
		var got int
		if err := sqlxDB.Get(&got, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName); err != nil {
			t.Fatalf("count %s: %v", indexName, err)
		}
		if got != want {
			t.Errorf("%s count = %d, want %d", indexName, got, want)
		}
	}
	ctx := context.Background()
	for _, id := range []string{"wf-replay-first", "wf-replay-second"} {
		if err := repo.CreateWorkflow(ctx, &models.Workflow{
			ID:                 id,
			WorkspaceID:        "ws-improve-kandev-index-replay",
			Name:               "Reusable template workflow",
			WorkflowTemplateID: strptr("reusable-template"),
			Hidden:             true,
		}); err != nil {
			t.Fatalf("create non-Improve-Kandev template workflow %q after replay: %v", id, err)
		}
	}
}

func TestImproveKandevWorkflowIndexReconcilesLegacyDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "improve-kandev-duplicates.db")
	openRepo := func() (*Repository, *sqlx.DB) {
		t.Helper()
		dbConn, err := db.OpenSQLite(dbPath)
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
		repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
		if err != nil {
			_ = sqlxDB.Close()
			t.Fatalf("new repo: %v", err)
		}
		return repo, sqlxDB
	}

	repo, sqlxDB := openRepo()
	workspaceID := "ws-improve-kandev-duplicates"
	seedWorkspace(t, repo, workspaceID)
	if _, err := sqlxDB.Exec(`DROP INDEX IF EXISTS uniq_improve_kandev_workflows`); err != nil {
		t.Fatalf("drop improve kandev index: %v", err)
	}
	ctx := context.Background()
	for _, id := range []string{"wf-legacy-first", "wf-legacy-second"} {
		if err := repo.CreateWorkflow(ctx, &models.Workflow{
			ID:                 id,
			WorkspaceID:        workspaceID,
			Name:               "Improve Kandev",
			WorkflowTemplateID: strptr("improve-kandev"),
			Hidden:             true,
		}); err != nil {
			t.Fatalf("create legacy duplicate %q: %v", id, err)
		}
	}
	if err := sqlxDB.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	repo, sqlxDB = openRepo()
	t.Cleanup(func() { _ = sqlxDB.Close() })
	workflows, err := repo.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		t.Fatalf("list reconciled workflows: %v", err)
	}
	var matchingTemplates int
	for _, workflow := range workflows {
		if workflow.WorkflowTemplateID != nil && *workflow.WorkflowTemplateID == "improve-kandev" {
			matchingTemplates++
		}
	}
	if matchingTemplates != 1 {
		t.Fatalf("improve-kandev workflow template rows = %d, want 1", matchingTemplates)
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

func TestRunnerProjectionWorkflowStepColumnsReplayMigration(t *testing.T) {
	repo := newRepoForEntityTests(t)
	if _, err := repo.db.Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES ('legacy-projection-step', 'legacy-workflow', 'Legacy', 0)`); err != nil {
		t.Fatalf("seed legacy workflow step: %v", err)
	}

	legacyColumns := []struct {
		name string
		sql  string
	}{
		{name: "auto_advance_requires_signal", sql: `ALTER TABLE workflow_steps DROP COLUMN auto_advance_requires_signal`},
		{name: "cancel_triggers_turn_complete", sql: `ALTER TABLE workflow_steps DROP COLUMN cancel_triggers_turn_complete`},
	}
	for _, column := range legacyColumns {
		if _, err := repo.db.Exec(column.sql); err != nil {
			t.Fatalf("drop legacy workflow_steps.%s: %v", column.name, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy workflow_steps schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}

	for _, column := range []string{"auto_advance_requires_signal", "cancel_triggers_turn_complete"} {
		var count int
		if err := repo.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workflow_steps') WHERE name = ?`, column).Scan(&count); err != nil {
			t.Fatalf("inspect workflow_steps.%s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("workflow_steps.%s column count = %d, want 1", column, count)
		}
	}
	var autoAdvance, cancelComplete int
	if err := repo.db.QueryRow(`SELECT auto_advance_requires_signal, cancel_triggers_turn_complete FROM workflow_steps WHERE id = 'legacy-projection-step'`).Scan(&autoAdvance, &cancelComplete); err != nil {
		t.Fatalf("read migrated workflow step: %v", err)
	}
	if autoAdvance != 0 || cancelComplete != 0 {
		t.Fatalf("legacy workflow step defaults = (%d, %d), want (0, 0)", autoAdvance, cancelComplete)
	}
}
