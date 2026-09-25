package executor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRepoSpecsCarriesTaskRepositoryIdentityAndIntegrationRef(t *testing.T) {
	specs := buildRepoSpecs([]*repoInfo{
		{RepositoryID: "repository-1", TaskRepositoryID: "task-repository-1", RepositoryPath: "/repo", IntegrationRef: "main"},
		{RepositoryID: "repository-1", TaskRepositoryID: "task-repository-2", RepositoryPath: "/repo", IntegrationRef: "release"},
	})

	require.Len(t, specs, 2)
	require.Equal(t, "task-repository-1", specs[0].TaskRepositoryID)
	require.Equal(t, "task-repository-2", specs[1].TaskRepositoryID)
	require.Equal(t, "main", specs[0].IntegrationRef)
	require.Equal(t, "release", specs[1].IntegrationRef)
}

func TestTopLevelLaunchRepoSpecCarriesExactTaskRepositoryID(t *testing.T) {
	spec, ok := topLevelLaunchRepoSpec(&LaunchAgentRequest{
		RepositoryID:     "repository-1",
		TaskRepositoryID: "task-repository-1",
		RepositoryPath:   "/repo",
		IntegrationRef:   "develop",
	})

	require.True(t, ok)
	require.Equal(t, "task-repository-1", spec.TaskRepositoryID)
	require.Equal(t, "develop", spec.IntegrationRef)
}
