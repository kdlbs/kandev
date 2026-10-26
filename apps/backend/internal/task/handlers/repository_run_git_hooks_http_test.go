package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func runGitHooksRequest(t *testing.T, router *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestHTTPCreateRepositoryRunGitHooks(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"absent stores off", `{"name":"a/one","source_type":"provider"}`, false},
		{"true is stored", `{"name":"a/two","source_type":"provider","run_git_hooks":true}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, repo := newRepositoryHTTPTestRouter(t)
			response := runGitHooksRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/repositories", tc.body)
			require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"run_git_hooks":`+map[bool]string{true: "true", false: "false"}[tc.want])
			repositories, err := repo.ListRepositories(context.Background(), "ws-1")
			require.NoError(t, err)
			require.Len(t, repositories, 1)
			require.Equal(t, tc.want, repositories[0].RunGitHooks)
		})
	}
}

func TestHTTPUpdateRepositoryRunGitHooks(t *testing.T) {
	cases := []struct {
		name   string
		stored bool
		body   string
		want   bool
	}{
		{"true is stored", false, `{"run_git_hooks":true}`, true},
		{"false clears a stored true", true, `{"run_git_hooks":false}`, false},
		{"null leaves a stored true", true, `{"run_git_hooks":null}`, true},
		{"other fields leave a stored true", true, `{"name":"renamed"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, repo := newRepositoryHTTPTestRouter(t)
			ctx := context.Background()
			require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
				ID: "repo-hooks", WorkspaceID: "ws-1", Name: "api", SourceType: "provider", RunGitHooks: tc.stored,
			}))
			response := runGitHooksRequest(t, router, http.MethodPatch, "/api/v1/repositories/repo-hooks", tc.body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			got, err := repo.GetRepository(ctx, "repo-hooks")
			require.NoError(t, err)
			require.Equal(t, tc.want, got.RunGitHooks)
		})
	}
}
