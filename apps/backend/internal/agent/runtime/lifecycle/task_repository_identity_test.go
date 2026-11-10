package lifecycle

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepoSpecsCarriesTaskRepositoryIDThroughLegacySynthesis(t *testing.T) {
	req := LaunchRequest{
		RepositoryID:     "repository-1",
		TaskRepositoryID: "task-repository-1",
		RepositoryPath:   "/repo",
	}

	specs := req.RepoSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "task-repository-1", specs[0].TaskRepositoryID)
}

func TestBuildEnvPrepareRequestCarriesTaskRepositoryIDs(t *testing.T) {
	req := &LaunchRequest{
		TaskID: "task-1",
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "repository-1", TaskRepositoryID: "task-repository-1", RepositoryPath: "/repo-1"},
			{RepositoryID: "repository-2", TaskRepositoryID: "task-repository-2", RepositoryPath: "/repo-2"},
		},
	}

	prepared := buildEnvPrepareRequest(req, "/workspace", "local")
	require.Len(t, prepared.Repositories, 2)
	require.Equal(t, "task-repository-1", prepared.Repositories[0].TaskRepositoryID)
	require.Equal(t, "task-repository-2", prepared.Repositories[1].TaskRepositoryID)
}

func TestWorkspaceRestorePreservesPlacementAndMCPRepositoryScopes(t *testing.T) {
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "repo-z", RepoName: "api", WorkspaceRelativePath: "repo/api", CheckoutBranch: "main"},
			{RepositoryID: "repo-a", RepoName: "lib", WorkspaceRelativePath: "repo/kandev/lib", CheckoutBranch: "main"},
			{RepositoryID: "repo-z", RepoName: "api-feature", WorkspaceRelativePath: "repo/api-feature", CheckoutBranch: "feature"},
		},
	}
	restored := workspaceRepositorySpecsFromLaunch(req)
	require.Equal(t, []string{"repo-a", "repo-z"}, repositoryIDsFromLaunch(req))
	require.Equal(t, []string{"repo-a", "repo-z"}, repositoryIDsFromWorkspace(restored))
	require.Len(t, restored, 3, "branch slots must remain distinct while MCP scopes are deduplicated")
	require.Equal(t, "repo/kandev/lib", restored[1].WorkspaceRelativePath)
	require.Equal(t, "feature", restored[2].CheckoutBranch)
	taskRoot := t.TempDir()
	for _, layout := range []string{"repository", "task_root"} {
		workspace := taskRoot
		if layout == "repository" {
			workspace = filepath.Join(taskRoot, "repo")
		}
		require.Equal(t, filepath.Join(taskRoot, "repo", "kandev", "lib"), workspaceRepositoryCandidate(
			workspace, layout, restored[1].RepoName, restored[1].WorkspaceRelativePath,
		))
	}
}
