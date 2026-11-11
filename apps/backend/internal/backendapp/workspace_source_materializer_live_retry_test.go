package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-TASKS-RUNTIME-CLEANUP-001.58
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
func TestWorkspaceSourceMaterializer_LiveAttachmentRetry(t *testing.T) {
	cases := []struct {
		name    string
		failure string
		single  bool
		rebuild bool
	}{
		{name: "inventory/live shared", failure: "second inventory insert"},
		{name: "inventory/live single", failure: "second inventory insert", single: true},
		{name: "inventory/rebuilt shared", failure: "second inventory insert", rebuild: true},
		{name: "rescan/live shared", failure: "second session rescan"},
		{name: "rescan/rebuilt shared", failure: "second session rescan", rebuild: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, taskRoot, root, scan := inventoryRollbackFixture(t)
			if tc.single {
				session, err := repo.GetTaskSession(ctx, "session-2")
				require.NoError(t, err)
				require.NoError(t, repo.DeleteTaskSession(ctx, session))
			}
			prior := snapshotLiveAttachment(t, repo)
			req := liveAttachmentRequest()
			configureInventoryFailure(t, repo, scan, tc.failure)
			preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
			require.NoError(t, err)
			req.PreviewRevision = preview.Revision
			_, err = svc.AttachWorkspaceSources(ctx, req)
			clearLiveAttachmentFailure(t, repo, scan, tc.failure, err)
			assertInventoryRollback(t, repo, prior.inventory, prior.sources, root, taskRoot, preview)
			sessions, err := repo.ListTaskSessions(ctx, req.TaskID)
			require.NoError(t, err)
			assertLiveAttachmentSessions(t, prior.sessions, sessions)
			require.Empty(t, scan.rebindCalls)
			if tc.rebuild {
				setInventoryServiceMaterializer(t, svc, repo, taskRoot, scan)
			}
			preview, err = svc.PreviewWorkspaceRepositoryPlacement(ctx, req.TaskID, req.Sources, req.RepositoryPlacement)
			require.NoError(t, err)
			req.PreviewRevision = preview.Revision
			result, err := svc.AttachWorkspaceSources(ctx, req)
			require.NoError(t, err, "retry through the selected manager must succeed after complete rollback")
			require.True(t, result.Changed)
			assertLiveAttachmentCommitted(t, repo, taskRoot, result.Task.Repositories)
			committed := snapshotLiveAttachment(t, repo)
			rescans := len(scan.rescanCalls)
			repeated, err := svc.AttachWorkspaceSources(ctx, req)
			require.NoError(t, err)
			require.False(t, repeated.Changed)
			require.Equal(t, committed, snapshotLiveAttachment(t, repo))
			require.Equal(t, rescans, len(scan.rescanCalls))
			require.Empty(t, scan.rebindCalls)
			assertLiveAttachmentSessions(t, prior.sessions, committed.sessions)
			require.Equal(t, root, committed.environment.WorkspacePath)
			require.Equal(t, "feature/initial", strings.TrimSpace(runGit(t, root, "branch", "--show-current")))
			assertLiveAttachmentReopened(t, repo, taskRoot, committed)
		})
	}
}

type liveAttachmentSnapshot struct {
	sources     []*models.TaskRepository
	inventory   []*models.TaskEnvironmentRepo
	environment *models.TaskEnvironment
	sessions    []*models.TaskSession
}

func assertLiveAttachmentSessions(t *testing.T, before, after []*models.TaskSession) {
	t.Helper()
	withoutInventory := func(sessions []*models.TaskSession) []models.TaskSession {
		result := make([]models.TaskSession, 0, len(sessions))
		for _, session := range sessions {
			snapshot := *session
			snapshot.Worktrees = nil
			result = append(result, snapshot)
		}
		return result
	}
	require.Equal(t, withoutInventory(before), withoutInventory(after))
}

func snapshotLiveAttachment(t *testing.T, repo *sqliterepo.Repository) liveAttachmentSnapshot {
	t.Helper()
	ctx := context.Background()
	sources, err := repo.ListTaskRepositories(ctx, "task-1")
	require.NoError(t, err)
	inventory, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	require.NoError(t, err)
	environment, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	require.NoError(t, err)
	sessions, err := repo.ListTaskSessions(ctx, "task-1")
	require.NoError(t, err)
	return liveAttachmentSnapshot{sources: sources, inventory: inventory, environment: environment, sessions: sessions}
}

func liveAttachmentRequest() taskservice.AttachWorkspaceSourcesRequest {
	return taskservice.AttachWorkspaceSourcesRequest{
		TaskID: "task-1", RepositoryPlacement: taskservice.WorkspacePlacementCurrentRoot,
		Sources: []taskservice.WorkspaceSourceInput{
			{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-2", BaseBranch: "main"},
			{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "repo-3", BaseBranch: "main"},
		},
	}
}

func clearLiveAttachmentFailure(t *testing.T, repo *sqliterepo.Repository, scan *failingWorkspaceRescanStub, failure string, err error) {
	t.Helper()
	if failure == "second inventory insert" {
		require.ErrorContains(t, err, "forced second inventory failure")
		_, dropErr := repo.DB().ExecContext(context.Background(), "DROP TRIGGER reject_second_attachment")
		require.NoError(t, dropErr)
		return
	}
	require.ErrorContains(t, err, "rescan failed")
	require.Len(t, scan.rescanCalls, 3)
	require.Equal(t, scan.rescanCalls[0], scan.rescanCalls[2])
	scan.failOnRescan = 0
}

func assertLiveAttachmentCommitted(t *testing.T, repo *sqliterepo.Repository, taskRoot string, sources []*models.TaskRepository) {
	t.Helper()
	batch := &models.WorkspaceSourceBatch{}
	for _, source := range sources {
		if source.ID != "tr-primary" {
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: source})
		}
	}
	require.Len(t, batch.Sources, 2)
	for _, row := range assertAttachmentInventory(t, repo, taskRoot, batch) {
		repository, err := repo.GetRepository(context.Background(), row.RepositoryID)
		require.NoError(t, err)
		registration := runGit(t, repository.LocalPath, "worktree", "list", "--porcelain")
		require.Contains(t, registration, "worktree "+filepath.ToSlash(row.WorktreePath)+"\n")
		require.Contains(t, registration, "branch refs/heads/"+row.WorktreeBranch+"\n")
		require.Equal(t, row.WorktreeBranch, strings.TrimSpace(runGit(t, row.WorktreePath, "branch", "--show-current")))
	}
}

func assertLiveAttachmentReopened(t *testing.T, repo *sqliterepo.Repository, taskRoot string, committed liveAttachmentSnapshot) {
	t.Helper()
	var seq int
	var name, path string
	require.NoError(t, repo.DB().QueryRowContext(context.Background(), "PRAGMA database_list").Scan(&seq, &name, &path))
	require.NotEmpty(t, path)
	require.NoError(t, repo.DB().Close())
	reopened := openAttachmentInventoryRepo(t, path)
	require.Equal(t, committed, snapshotLiveAttachment(t, reopened))
	assertLiveAttachmentCommitted(t, reopened, taskRoot, committed.sources)
	content, err := os.ReadFile(filepath.Join(committed.environment.WorkspacePath, "keep.txt"))
	require.NoError(t, err)
	require.Equal(t, "unrelated", string(content))
}
