package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMockControllerRepositoryPermissionFixtures(t *testing.T) {
	router, service, _ := setupWorkspaceAuthMockController(t)
	response := serveMockJSON(t, router, http.MethodPut, "/api/v1/github/mock/repository-details",
		`{"owner":"upstream","repo":"site","push_access":false}`)
	require.Equal(t, http.StatusOK, response.Code)
	client := service.client.(*MockClient)
	repository, err := client.GetRepository(context.Background(), "upstream", "site")
	require.NoError(t, err)
	require.True(t, repository.PushAccessKnown)
	require.False(t, repository.PushAccess)
	// Metadata alone must not broaden the workspace's readable repository scope.
	visible, err := client.HasRepositoryAccess(context.Background(), "upstream", "site")
	require.NoError(t, err)
	require.False(t, visible)
	response = serveMockJSON(t, router, http.MethodPut, "/api/v1/github/mock/repository-details",
		`{"owner":"upstream","repo":"site","push_access":null}`)
	require.Equal(t, http.StatusOK, response.Code)
	repository, err = client.GetRepository(context.Background(), "upstream", "site")
	require.NoError(t, err)
	require.False(t, repository.PushAccessKnown)
}
