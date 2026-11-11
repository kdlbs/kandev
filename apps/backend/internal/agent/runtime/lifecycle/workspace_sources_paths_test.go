package lifecycle

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceSourceRootsSkipsEmptyPaths(t *testing.T) {
	primary := t.TempDir()
	roots := workspaceSourceRoots(nil, []WorkspaceRepositorySpec{
		{RepositoryPath: primary},
		{RepositoryPath: "", WorktreePath: ""},
	})
	require.Equal(t, []string{primary}, roots)
}

func TestWorkspaceRepositorySpecsFromLaunchCarriesAdoptedPath(t *testing.T) {
	req := &LaunchRequest{
		ExecutorType:  "worktree",
		WorkspacePath: "/tasks/task-1/primary",
		TaskDirName:   "task-1",
		Repositories: []RepoLaunchSpec{{
			RepositoryID:          "repo-1",
			RepositoryPath:        "/repos/repo-1",
			WorkspaceRelativePath: ".claude/worktrees/fix-b",
		}},
	}

	specs := workspaceRepositorySpecsFromLaunch(req)
	require.Len(t, specs, 1)
	require.Equal(t, filepath.Join("/tasks/task-1", ".claude/worktrees/fix-b"), specs[0].WorktreePath)
}

func TestWorkspaceRepositorySpecsFromLaunchDerivesTaskRootPaths(t *testing.T) {
	req := &LaunchRequest{
		ExecutorType:  "worktree",
		WorkspacePath: "/tasks/task-1",
		TaskDirName:   "task-1",
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "repo-a", RepositoryPath: "/repos/a", RepoName: "api"},
			{RepositoryID: "repo-b", RepositoryPath: "/repos/b", RepoName: "web", BranchSlug: "feature-b"},
		},
	}

	specs := workspaceRepositorySpecsFromLaunch(req)
	require.Len(t, specs, 2)
	require.Equal(t, filepath.Join("/tasks/task-1", "api"), specs[0].WorktreePath)
	require.Equal(t, filepath.Join("/tasks/task-1", "web-feature-b"), specs[1].WorktreePath)
}
