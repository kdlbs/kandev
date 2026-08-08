package gitlab

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoContentsMockService wires a working per-workspace connection whose
// resolved client is a seedable MockClient, so tests exercise real
// credential/host resolution without hitting the network.
func repoContentsMockService(t *testing.T, workspaceID string) (*Service, *MockClient) {
	t.Helper()
	_, _, svc, _ := setupWorkingWorkspaceConfig(t, workspaceID)
	mock := NewMockClient("https://working.gitlab.example")
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	return svc, mock
}

func TestService_ListRepoTreeForWorkspace(t *testing.T) {
	svc, mock := repoContentsMockService(t, "ws-1")
	mock.SeedRepoFile("acme/project", "main", ".kandev/workflows/review.yaml", []byte("name: Review\n"))

	entries, err := svc.ListRepoTreeForWorkspace(context.Background(), "ws-1", "acme/project", ".kandev/workflows", "main")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "review.yaml", entries[0].Name)
}

func TestService_GetRepoFileContentForWorkspace(t *testing.T) {
	svc, mock := repoContentsMockService(t, "ws-1")
	mock.SeedRepoFile("acme/project", "main", ".kandev/workflows/review.yaml", []byte("name: Review\n"))

	content, err := svc.GetRepoFileContentForWorkspace(
		context.Background(), "ws-1", "acme/project", ".kandev/workflows/review.yaml", "main")
	require.NoError(t, err)
	assert.Equal(t, "name: Review\n", string(content))
}

// A workspace with no GitLab connection must fail clearly, not panic or
// silently return an empty result.
func TestService_RepoContentsForWorkspace_NoConnection(t *testing.T) {
	store := newTestStore(t)
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: map[string]string{}})

	_, err := svc.ListRepoTreeForWorkspace(context.Background(), "ws-unconfigured", "acme/project", "", "main")
	require.Error(t, err)

	_, err = svc.GetRepoFileContentForWorkspace(context.Background(), "ws-unconfigured", "acme/project", "a.yaml", "main")
	require.Error(t, err)
}

func TestService_ListRepoTreeForWorkspace_RequiresWorkspaceID(t *testing.T) {
	store := newTestStore(t)
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: map[string]string{}})

	_, err := svc.ListRepoTreeForWorkspace(context.Background(), "", "acme/project", "", "main")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkspaceRequired))
}
