package github

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.15
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.17
func TestController_RepoMergeAccess(t *testing.T) {
	for _, tt := range []struct {
		name, permissions, want string
	}{
		{"writer", `{"push":true}`, "allowed"},
		{"reader", `{"push":false,"admin":false}`, "denied"},
		{"missing push", `{"admin":true}`, "unknown"},
		{"null push", `{"push":null}`, "unknown"},
		{"missing permissions", `null`, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			client.AddRepos("acme", []GitHubRepo{{Owner: "acme", Name: "site", FullName: "acme/site"}})
			var raw githubRepositoryResponse
			require.NoError(t, json.Unmarshal([]byte(`{"full_name":"acme/site","owner":{"login":"acme"},"name":"site","permissions":`+tt.permissions+`}`), &raw))
			client.SetRepositoryDetails(*projectGitHubRepository(raw))
			service := newTestService(client)
			configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
			router := gin.New()
			NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				"/api/v1/github/repos/acme/site/merge-access?workspace_id="+testWorkspaceID, nil))

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			var body struct {
				State string        `json:"state"`
				Actor AuthPrincipal `json:"actor"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, tt.want, body.State)
			require.Equal(t, "test-user", body.Actor.Login)
			require.Equal(t, testWorkspaceID, body.Actor.WorkspaceID)
		})
	}
}

func TestController_RepoMergeAccessUnavailableDoesNotExposeProviderErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{"unavailable", errors.New("secret-provider-diagnostic"), http.StatusServiceUnavailable},
		{"forbidden", &GitHubAPIError{StatusCode: http.StatusForbidden, Endpoint: "secret-provider-diagnostic"}, http.StatusForbidden},
		{"not found", &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "secret-provider-diagnostic"}, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := mergeAccessReadClient{MockClient: NewMockClient(), err: tt.err}
			client.AddRepos("acme", []GitHubRepo{{Owner: "acme", Name: "site"}})
			service := newTestService(client)
			configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
			router := gin.New()
			NewController(service, testLogger(t)).RegisterHTTPRoutes(router)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				"/api/v1/github/repos/acme/site/merge-access?workspace_id="+testWorkspaceID, nil))
			require.Equal(t, tt.status, response.Code)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			require.NotContains(t, response.Body.String(), "secret-provider-diagnostic")
		})
	}
}
