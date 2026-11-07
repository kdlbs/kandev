package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorktreePreparerPreservesPhysicalPathWithParentWorkspace(t *testing.T) {
	preparer, _, store := newPreparerForTestWithStore(t)
	req := &EnvPrepareRequest{
		TaskID: "task-parent", SessionID: "session-parent", TaskTitle: "Parent workspace",
		TaskEnvironmentID: "environment-parent",
		TaskDirName:       "parent-workspace_aaa", WorkspaceLayout: "task_root",
		RepositoryID: "repo-parent", RepositoryPath: initBareGitRepo(t, "parent-repo"),
		RepoName: "parent-repo", BaseBranch: "main", IntegrationRef: "main",
	}
	for _, reuse := range []bool{false, true} {
		req.WorkspaceReuseRequired = reuse
		result, err := preparer.Prepare(context.Background(), req, nil)
		require.NoError(t, err)
		require.True(t, result.Success, result.ErrorMessage)
		wt, err := store.GetWorktreeByID(context.Background(), result.WorktreeID)
		require.NoError(t, err)
		require.NotNil(t, wt)
		require.Equal(t, filepath.Dir(wt.Path), result.WorkspacePath)
		require.Equal(t, wt.Path, result.WorktreePath)
		req.WorktreeID = wt.ID
	}
}

func TestStandaloneExecutorPreservesPhysicalPathWithParentWorkspace(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	instance, err := control.executor(t).CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-parent", TaskID: "task-parent", SessionID: "session-parent",
		WorkspacePath: "/tasks/parent",
		Metadata: map[string]interface{}{
			MetadataKeyWorktreeID: "worktree-parent", "worktree_path": "/tasks/parent/repo",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "/tasks/parent", control.lastCreateRequest(t).WorkspacePath)
	require.Equal(t, "/tasks/parent", instance.WorkspacePath)
	require.Equal(t, "/tasks/parent/repo", instance.Metadata["worktree_path"])
}
