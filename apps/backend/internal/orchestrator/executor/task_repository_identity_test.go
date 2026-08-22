package executor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRepoSpecsCarriesExactTaskRepositoryID(t *testing.T) {
	specs := buildRepoSpecs([]*repoInfo{
		{RepositoryID: "repository-1", TaskRepositoryID: "task-repository-1", RepositoryPath: "/repo"},
		{RepositoryID: "repository-1", TaskRepositoryID: "task-repository-2", RepositoryPath: "/repo"},
	})

	require.Len(t, specs, 2)
	require.Equal(t, "task-repository-1", specs[0].TaskRepositoryID)
	require.Equal(t, "task-repository-2", specs[1].TaskRepositoryID)
}

func TestTopLevelLaunchRepoSpecCarriesExactTaskRepositoryID(t *testing.T) {
	spec, ok := topLevelLaunchRepoSpec(&LaunchAgentRequest{
		RepositoryID:     "repository-1",
		TaskRepositoryID: "task-repository-1",
		RepositoryPath:   "/repo",
	})

	require.True(t, ok)
	require.Equal(t, "task-repository-1", spec.TaskRepositoryID)
}
