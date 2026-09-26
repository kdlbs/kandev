package lifecycle

import (
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
