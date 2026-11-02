package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func sourcePlacementQAService(t *testing.T, executor string) (*Service, *sqliterepo.Repository, *models.TaskEnvironment) {
	t.Helper()
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-qa", Name: "QA"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-qa", WorkspaceID: "ws-qa", Name: "QA"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-qa", WorkspaceID: "ws-qa", WorkflowID: "wf-qa", Title: "QA"}))
	env := &models.TaskEnvironment{ID: "env-qa", TaskID: "task-qa", ExecutorType: executor, Status: models.TaskEnvironmentStatusReady, TaskDirName: "task-qa", WorkspacePath: filepath.Join(t.TempDir(), "primary"), WorkspaceLayout: WorkspaceLayoutRepository}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
	return svc, repo, env
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.2
func TestPreviewWorkspaceSourcesRawLocatorsAreReadOnlyAndMatchAttachment(t *testing.T) {
	for _, executor := range []string{"local", "worktree"} {
		for _, kind := range []string{"local", "remote", "saved-path"} {
			t.Run(executor+"/"+kind, func(t *testing.T) {
				svc, repo, env := sourcePlacementQAService(t, executor)
				ctx := context.Background()
				input := WorkspaceSourceInput{Kind: WorkspaceSourceRepository, BaseBranch: "main"}
				if kind == "remote" {
					input.RemoteURL = "https://github.com/acme/tools.git"
				} else {
					input.LocalPath = filepath.Join(t.TempDir(), "tools")
					seedBareGitDir(t, input.LocalPath, "ref: refs/heads/main\n")
				}
				if kind == "saved-path" {
					require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "saved-tools", WorkspaceID: "ws-qa", Name: "Custom tools", LocalPath: canonicalRepoTestPath(t, input.LocalPath), DefaultBranch: "main"}))
				}
				sources := []WorkspaceSourceInput{input, {Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "notes"}}
				before, err := repo.ListRepositories(ctx, "ws-qa")
				require.NoError(t, err)
				preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, "task-qa", sources, WorkspacePlacementKandevDirectory)
				require.NoError(t, err)
				require.Equal(t, env.WorkspacePath, preview.WorkspacePath)
				after, err := repo.ListRepositories(ctx, "ws-qa")
				require.NoError(t, err)
				require.Equal(t, len(before), len(after), "preview must not register repositories")
				result, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: "task-qa", Sources: sources, RepositoryPlacement: WorkspacePlacementKandevDirectory, PreviewRevision: preview.Revision})
				require.NoError(t, err)
				require.True(t, result.Changed)
				attached, err := repo.ListTaskRepositories(ctx, "task-qa")
				require.NoError(t, err)
				folders, err := repo.ListTaskWorkspaceFolders(ctx, "task-qa")
				require.NoError(t, err)
				require.Equal(t, preview.Sources[0].WorkspaceRelativePath, attached[0].WorkspaceRelativePath)
				require.Equal(t, "kandev/notes", folders[0].WorkspaceRelativePath)
				retry, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: "task-qa", Sources: sources, RepositoryPlacement: WorkspacePlacementKandevDirectory, PreviewRevision: preview.Revision})
				require.NoError(t, err)
				require.False(t, retry.Changed)
			})
		}
	}
}

func TestAttachWorkspaceSourcesOmittedPlacementPreservesRootDestinations(t *testing.T) {
	for _, status := range []models.TaskEnvironmentStatus{models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped} {
		t.Run(string(status), func(t *testing.T) {

			svc, repo, env := sourcePlacementQAService(t, "worktree")
			ctx := context.Background()
			env.Status = status
			require.NoError(t, repo.UpdateTaskEnvironment(ctx, env))
			sourcePath := filepath.Join(t.TempDir(), "tools")
			seedBareGitDir(t, sourcePath, "ref: refs/heads/main\n")
			_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: "task-qa", Sources: []WorkspaceSourceInput{
				{Kind: WorkspaceSourceRepository, LocalPath: sourcePath, BaseBranch: "main"},
				{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "notes"},
			}})
			require.NoError(t, err)
			repos, err := repo.ListTaskRepositories(ctx, "task-qa")
			require.NoError(t, err)
			require.Equal(t, filepath.ToSlash(filepath.Join(filepath.Base(env.WorkspacePath), "tools-main")), repos[0].WorkspaceRelativePath)
			folders, err := repo.ListTaskWorkspaceFolders(ctx, "task-qa")
			require.NoError(t, err)
			require.Equal(t, "notes", folders[0].WorkspaceRelativePath)

		})
	}
}

func TestAttachWorkspaceSourcesStaleRawLocatorLeavesNoRepository(t *testing.T) {
	svc, repo, _ := sourcePlacementQAService(t, "local")
	ctx := context.Background()
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: "task-qa", RepositoryPlacement: WorkspacePlacementCurrentRoot, PreviewRevision: "stale", Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RemoteURL: "https://github.com/acme/tools.git", BaseBranch: "main"}}})
	require.ErrorIs(t, err, ErrWorkspaceSourcePreviewStale)
	repos, err := repo.ListRepositories(ctx, "ws-qa")
	require.NoError(t, err)
	require.Empty(t, repos, "rejected placement must roll back repository registration")
}
