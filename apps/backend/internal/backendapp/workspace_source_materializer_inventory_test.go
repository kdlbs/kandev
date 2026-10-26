package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
func TestWorkspaceSourceMaterializer_PersistedAttachmentPlacement(t *testing.T) {
	cases := []struct {
		name       string
		placement  taskservice.WorkspaceRepositoryPlacement
		parentRoot bool
		prefix     string
	}{
		{name: "current root", placement: taskservice.WorkspacePlacementCurrentRoot, prefix: "kandev"},
		{name: "kandev directory", placement: taskservice.WorkspacePlacementKandevDirectory, prefix: "kandev/kandev"},
		{name: "parent root", placement: taskservice.WorkspacePlacementCurrentRoot, parentRoot: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
			dbPath := filepath.Join(t.TempDir(), "inventory.db")
			repo := openAttachmentInventoryRepo(t, dbPath)
			seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)
			root := primaryPath
			if tc.parentRoot {
				root = taskRoot
				env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
				require.NoError(t, err)
				env.WorkspacePath, env.WorkspaceLayout = root, taskservice.WorkspaceLayoutTaskRoot
				require.NoError(t, repo.UpdateTaskEnvironment(ctx, env))
			}
			secondary := filepath.Join(t.TempDir(), "secondary")
			runGit(t, repoPath, "clone", repoPath, secondary)
			require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
				ID: "repo-2", WorkspaceID: "ws-1", Name: "secondary", LocalPath: secondary, DefaultBranch: "main",
			}))
			batch := inventoryPlacementBatch(t, repo, tc.prefix, tc.placement)
			mgr := newPersistingMaterializerWorktreeMgr(t, taskRoot, repo)
			rescanner := &nestedWorkspaceRescanStub{}
			m := &workspaceSourceMaterializer{
				repo: repo, worktreeMgr: mgr, rescanner: rescanner, logger: newTestLogger(),
				branches: &branchMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescanner, logger: newTestLogger()},
			}
			result, err := m.MaterializeWorkspaceSources(ctx, "task-1", batch)
			require.NoError(t, err)
			require.Equal(t, root, result.WorkspacePath)
			require.Empty(t, rescanner.rebindCalls)
			rows := assertAttachmentInventory(t, repo, taskRoot, batch)
			for _, row := range rows {
				require.NoError(t, os.WriteFile(filepath.Join(row.WorktreePath, "retained.txt"), []byte(row.WorktreeID), 0o600))
			}
			require.NoError(t, repo.DB().Close())
			reopened := openAttachmentInventoryRepo(t, dbPath)
			require.Equal(t, rows, assertAttachmentInventory(t, reopened, taskRoot, batch))
			fresh := newPersistingMaterializerWorktreeMgr(t, taskRoot, reopened)
			branches := &branchMaterializer{repo: reopened, worktreeMgr: fresh, logger: newTestLogger()}
			for index, row := range rows {
				materialized, err := branches.materializeUnfinalized(ctx, "task-1", batch.Sources[index].Repository.ID, nil)
				require.NoError(t, err)
				require.NotNil(t, materialized)
				wt := materialized.worktree
				require.Equal(t, row.WorktreeID, wt.ID)
				require.Equal(t, row.WorktreePath, wt.Path)
				require.Equal(t, row.WorkspaceRelativePath, wt.WorkspaceRelativePath)
				require.Equal(t, row.Position, wt.Position)
				content, err := os.ReadFile(filepath.Join(wt.Path, "retained.txt"))
				require.NoError(t, err)
				require.Equal(t, row.WorktreeID, string(content))
			}
			require.Equal(t, rows, assertAttachmentInventory(t, reopened, taskRoot, batch))
		})
	}
}

func openAttachmentInventoryRepo(t *testing.T, path string) *sqliterepo.Repository {
	t.Helper()
	conn, err := dbutil.OpenSQLite(path)
	require.NoError(t, err)
	db := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqliterepo.NewWithDB(db, db, newTestLogger())
	require.NoError(t, err)
	return repo
}

func inventoryPlacementBatch(t *testing.T, repo *sqliterepo.Repository, prefix string, placement taskservice.WorkspaceRepositoryPlacement) *models.WorkspaceSourceBatch {
	t.Helper()
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", RepositoryPlacement: string(placement)}
	for index, repositoryID := range []string{"repo-2", "repo-1", "repo-1"} {
		name := []string{"new-repository", "branch-one", "branch-two"}[index]
		tr := &models.TaskRepository{
			ID: name, TaskID: "task-1", RepositoryID: repositoryID,
			WorkspaceRelativePath: filepath.ToSlash(filepath.Join(prefix, name)),
			BaseBranch:            "main", CheckoutBranch: name, Position: index + 1,
		}
		require.NoError(t, repo.CreateTaskRepository(context.Background(), tr))
		batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: tr})
	}
	return batch
}

func assertAttachmentInventory(t *testing.T, repo *sqliterepo.Repository, taskRoot string, batch *models.WorkspaceSourceBatch, additional ...*models.TaskEnvironmentRepo) []*models.TaskEnvironmentRepo {
	t.Helper()
	rows, err := repo.ListTaskEnvironmentRepos(context.Background(), "env-1")
	require.NoError(t, err)
	require.Len(t, rows, len(batch.Sources)+1+len(additional))
	result := make([]*models.TaskEnvironmentRepo, 0, len(batch.Sources))
	for _, source := range batch.Sources {
		tr := source.Repository
		var matches []*models.TaskEnvironmentRepo
		for _, row := range rows {
			if row.RepositoryID == tr.RepositoryID && row.BranchSlug == workspaceSourceBranchSlug(tr) {
				matches = append(matches, row)
			}
		}
		require.Len(t, matches, 1)
		row := matches[0]
		expectedPath := filepath.Join(taskRoot, filepath.FromSlash(tr.WorkspaceRelativePath))
		require.DirExists(t, expectedPath)
		require.Equal(t, tr.WorkspaceRelativePath, row.WorkspaceRelativePath, "canonical path for %s", tr.ID)
		require.Equal(t, tr.Position, row.Position, "canonical position for %s", tr.ID)
		require.Equal(t, expectedPath, row.WorktreePath)
		require.NotEmpty(t, row.WorktreeID)
		require.NotEmpty(t, row.WorktreeBranch)
		require.Equal(t, "active", row.Status)
		require.Nil(t, row.DeletedAt)
		result = append(result, row)
	}
	return result
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
func TestWorkspaceSourceMaterializer_PersistedAttachmentRollbackAndRetry(t *testing.T) {
	for _, failure := range []string{"second inventory insert", "second session rescan"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, taskRoot, root, scan := inventoryRollbackFixture(t)
			before, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
			require.NoError(t, err)
			sources, err := repo.ListTaskRepositories(ctx, "task-1")
			require.NoError(t, err)
			req := taskservice.AttachWorkspaceSourcesRequest{
				TaskID: "task-1", RepositoryPlacement: taskservice.WorkspacePlacementCurrentRoot,
				Sources: []taskservice.WorkspaceSourceInput{
					{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-2", BaseBranch: "main"},
					{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-3", BaseBranch: "main"},
				},
			}
			configureInventoryFailure(t, repo, scan, failure)
			preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
			require.NoError(t, err)
			req.PreviewRevision = preview.Revision
			_, err = svc.AttachWorkspaceSources(ctx, req)
			require.Error(t, err)
			if failure == "second inventory insert" {
				require.ErrorContains(t, err, "forced second inventory failure")
				_, err = repo.DB().ExecContext(ctx, "DROP TRIGGER reject_second_attachment")
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "rescan failed")
				require.Len(t, scan.rescanCalls, 3)
				require.Equal(t, scan.rescanCalls[0], scan.rescanCalls[2])
			}
			assertInventoryRollback(t, repo, before, sources, root, taskRoot, preview)
			require.Empty(t, scan.rebindCalls)
			// Retry after manager reconstruction models the backend restart boundary.
			// Live multi-session cache eviction is a separate, deferred defect.
			setInventoryServiceMaterializer(t, svc, repo, taskRoot, scan)
			preview, err = svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
			require.NoError(t, err)
			req.PreviewRevision = preview.Revision
			result, err := svc.AttachWorkspaceSources(ctx, req)
			require.NoError(t, err)
			require.True(t, result.Changed)
			batch := &models.WorkspaceSourceBatch{}
			for _, tr := range result.Task.Repositories {
				if tr.ID != "tr-primary" {
					batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: tr})
				}
			}
			rows := assertAttachmentInventory(t, repo, taskRoot, batch)
			calls := len(scan.rescanCalls)
			retry, err := svc.AttachWorkspaceSources(ctx, req)
			require.NoError(t, err)
			require.False(t, retry.Changed)
			require.Equal(t, calls, len(scan.rescanCalls))
			require.Equal(t, rows, assertAttachmentInventory(t, repo, taskRoot, batch))
			require.Empty(t, scan.rebindCalls)
		})
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
func TestWorkspaceSourceMaterializer_PersistedAttachmentChangedPlacementRetry(t *testing.T) {
	ctx := context.Background()
	svc, repo, taskRoot, root, scan := inventoryRollbackFixture(t)
	before, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	sources, err := repo.ListTaskRepositories(ctx, "task-1")
	require.NoError(t, err)
	foreign := &models.TaskEnvironmentRepo{
		ID: "foreign-inactive", TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		BranchSlug: "foreign", WorkspaceRelativePath: "foreign/location", Position: 19, Status: "failed",
	}
	require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, foreign))
	req := taskservice.AttachWorkspaceSourcesRequest{
		TaskID: "task-1", RepositoryPlacement: taskservice.WorkspacePlacementCurrentRoot,
		Sources: []taskservice.WorkspaceSourceInput{
			{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-2", BaseBranch: "main"},
			{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-3", BaseBranch: "main"},
		},
	}
	configureInventoryFailure(t, repo, scan, "second session rescan")
	oldPreview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
	require.NoError(t, err)
	req.PreviewRevision = oldPreview.Revision
	_, err = svc.AttachWorkspaceSources(ctx, req)
	require.ErrorContains(t, err, "rescan failed")
	assertInventoryRollback(t, repo, before, sources, root, taskRoot, oldPreview)
	require.Empty(t, scan.rebindCalls)

	// Retry after manager reconstruction admits a new placement and repository order.
	setInventoryServiceMaterializer(t, svc, repo, taskRoot, scan)
	req.RepositoryPlacement = taskservice.WorkspacePlacementKandevDirectory
	req.Sources[0], req.Sources[1] = req.Sources[1], req.Sources[0]
	preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
	require.NoError(t, err)
	req.PreviewRevision = preview.Revision
	result, err := svc.AttachWorkspaceSources(ctx, req)
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Len(t, result.Task.Repositories, 3)
	batch := &models.WorkspaceSourceBatch{}
	for _, tr := range result.Task.Repositories {
		if tr.ID != "tr-primary" {
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: tr})
		}
	}
	require.Equal(t, "repo-3", batch.Sources[0].Repository.RepositoryID)
	require.Equal(t, 1, batch.Sources[0].Repository.Position)
	require.Equal(t, "repo-2", batch.Sources[1].Repository.RepositoryID)
	require.Equal(t, 2, batch.Sources[1].Repository.Position)
	calls := len(scan.rescanCalls)
	retry, err := svc.AttachWorkspaceSources(ctx, req)
	require.NoError(t, err)
	require.False(t, retry.Changed)
	require.Equal(t, calls, len(scan.rescanCalls))
	require.Empty(t, scan.rebindCalls)
	persisted, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	var seq int
	var databaseName, databasePath string
	require.NoError(t, repo.DB().QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &databaseName, &databasePath))
	require.NoError(t, repo.DB().Close())
	reopened := openAttachmentInventoryRepo(t, databasePath)
	rows, err := reopened.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	require.Equal(t, persisted, rows)
	assertAttachmentInventory(t, reopened, taskRoot, batch, foreign)
	require.Len(t, rows, len(before)+len(batch.Sources)+1)
	for _, prior := range append(before, foreign) {
		require.Contains(t, rows, prior)
	}
	for _, source := range oldPreview.Sources {
		require.NoDirExists(t, filepath.Join(taskRoot, filepath.FromSlash(source.WorkspaceRelativePath)))
	}
	content, err := os.ReadFile(filepath.Join(root, "keep.txt"))
	require.NoError(t, err)
	require.Equal(t, "unrelated", string(content))
	environment, err := reopened.GetTaskEnvironmentByTaskID(ctx, "task-1")
	require.NoError(t, err)
	require.Equal(t, root, environment.WorkspacePath)
	require.Equal(t, string(req.RepositoryPlacement), environment.WorkspaceLayout)
	require.Equal(t, "feature/initial", strings.TrimSpace(runGit(t, root, "branch", "--show-current")))
}

func inventoryRollbackFixture(t *testing.T) (*taskservice.Service, *sqliterepo.Repository, string, string, *failingWorkspaceRescanStub) {
	t.Helper()
	ctx := context.Background()
	repoPath, taskRoot, root := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, root)
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	rows[0].WorktreeID, rows[0].WorktreeBranch = "primary-worktree", "feature/initial"
	rows[0].WorkspaceRelativePath, rows[0].BranchSlug = "kandev", "main"
	require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, rows[0]))
	_, err = repo.DB().ExecContext(ctx, `UPDATE task_repositories SET workspace_relative_path='kandev' WHERE id='tr-primary'`)
	require.NoError(t, err)
	for _, id := range []string{"repo-2", "repo-3"} {
		path := filepath.Join(t.TempDir(), id)
		runGit(t, repoPath, "clone", repoPath, path)
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, LocalPath: path, DefaultBranch: "main",
		}))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "keep.txt"), []byte("unrelated"), 0o600))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-2", TaskID: "task-1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo, Workflows: repo,
		RepoEntities: repo, Sessions: repo, Turns: repo, Messages: repo, TaskEnvironments: repo,
	}, eventBus, newTestLogger(), taskservice.RepositoryDiscoveryConfig{})
	scan := &failingWorkspaceRescanStub{}
	setInventoryServiceMaterializer(t, svc, repo, taskRoot, scan)
	return svc, repo, taskRoot, root, scan
}

func setInventoryServiceMaterializer(t *testing.T, svc *taskservice.Service, repo *sqliterepo.Repository, taskRoot string, scan *failingWorkspaceRescanStub) {
	t.Helper()
	mgr := newPersistingMaterializerWorktreeMgr(t, taskRoot, repo)
	svc.SetWorkspaceSourceMaterializer(&workspaceSourceMaterializer{
		repo: repo, worktreeMgr: mgr, rescanner: scan, logger: newTestLogger(),
		branches: &branchMaterializer{repo: repo, worktreeMgr: mgr, rescanner: scan, logger: newTestLogger()},
	})
}

func configureInventoryFailure(t *testing.T, repo *sqliterepo.Repository, scan *failingWorkspaceRescanStub, failure string) {
	t.Helper()
	if failure == "second session rescan" {
		scan.failOnRescan = 2
		return
	}
	_, err := repo.DB().ExecContext(context.Background(), `CREATE TRIGGER reject_second_attachment
        BEFORE INSERT ON task_environment_repos
        WHEN NEW.repository_id='repo-3' AND EXISTS (
            SELECT 1 FROM task_environment_repos WHERE repository_id='repo-2' AND status='active' AND worktree_id <> '')
        BEGIN SELECT RAISE(FAIL, 'forced second inventory failure'); END`)
	require.NoError(t, err)
}

func assertInventoryRollback(t *testing.T, repo *sqliterepo.Repository, before []*models.TaskEnvironmentRepo, sources []*models.TaskRepository, root, taskRoot string, preview *taskservice.WorkspaceRepositoryPlacementPreview) {
	t.Helper()
	ctx := context.Background()
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	var active []*models.TaskEnvironmentRepo
	for _, row := range rows {
		if row.Status == "active" && row.DeletedAt == nil {
			active = append(active, row)
		}
	}
	require.Equal(t, before, active)
	after, err := repo.ListTaskRepositories(ctx, "task-1")
	require.NoError(t, err)
	require.Equal(t, sources, after)
	for _, source := range preview.Sources {
		require.NoDirExists(t, filepath.Join(taskRoot, filepath.FromSlash(source.WorkspaceRelativePath)))
	}
	environment, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	require.NoError(t, err)
	require.Equal(t, root, environment.WorkspacePath)
	require.Empty(t, environment.WorkspaceLayout)
	content, err := os.ReadFile(filepath.Join(root, "keep.txt"))
	require.NoError(t, err)
	require.Equal(t, "unrelated", string(content))
	require.Equal(t, "feature/initial", strings.TrimSpace(runGit(t, root, "branch", "--show-current")))
	require.Equal(t, "?? keep.txt", strings.TrimSpace(runGit(t, root, "status", "--porcelain")))
	require.NotContains(t, runGit(t, root, "config", "--local", "--list"), "includeif.")
}
