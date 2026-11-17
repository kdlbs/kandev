package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Scope rejections have a distinct public code without returning the requested
// repository identity or the raw internal error.
func TestWriteGitHubOperationalAuthErrorScopeRejectionKeepsSanitized404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if !writeGitHubOperationalAuthError(ctx, newRepoOutsideScopeErr("acme", "widget")) {
		t.Fatal("scope rejection must be handled by the shared mapping, not fall through to a raw 500")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "github_repository_out_of_scope" {
		t.Fatalf("code = %q, want github_repository_out_of_scope", body.Code)
	}
	if !strings.Contains(body.Error, "GitHub repository scope") {
		t.Fatalf("error must name the real cause, got %q", body.Error)
	}
	// Inspect the full response, including fields outside the decoded struct.
	for _, leaked := range []string{"acme", "widget", "outside this workspace's scope"} {
		if strings.Contains(recorder.Body.String(), leaked) {
			t.Fatalf("scope rejection body leaked %q: %s", leaked, recorder.Body.String())
		}
	}
}

// Unresolvable repositories retain their existing public code and status.
func TestWriteGitHubOperationalAuthErrorUnresolvableKeeps404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if !writeGitHubOperationalAuthError(ctx, ErrRepoNotResolvable) {
		t.Fatal("unresolvable repository must be handled by the shared mapping")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "github_repository_not_found" {
		t.Fatalf("code = %q, want github_repository_not_found", body.Code)
	}
}

func TestRepositoryEndpointsRejectOutsideWorkspaceScope(t *testing.T) {
	for _, endpoint := range []string{"branches", "merge-methods"} {
		t.Run(endpoint, func(t *testing.T) {
			store := newTestStore(t)
			seedConnectionWorkspaces(t, store, "ws-1")
			if err := store.UpsertWorkspaceSettings(context.Background(), &WorkspaceSettings{
				WorkspaceID:   "ws-1",
				RepoScopeMode: RepoScopeModeOrgs,
				RepoScopeOrgs: []string{"private-org"},
			}); err != nil {
				t.Fatalf("store workspace scope: %v", err)
			}
			router, controller := setupControllerTest(&stubClient{})
			controller.service.store = store
			controller.service.resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
				t.Fatal("scope rejection must precede credential lookup")
				return nil, AuthMethodNone, nil
			})

			request := httptest.NewRequest(http.MethodGet,
				"/api/v1/github/repos/acme/widget/"+endpoint+"?workspace_id=ws-1", nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404: %s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != "github_repository_out_of_scope" {
				t.Fatalf("code = %q, want github_repository_out_of_scope", body.Code)
			}
			if !strings.Contains(body.Error, "GitHub repository scope") {
				t.Fatalf("error must identify the scope rejection, got %q", body.Error)
			}
			for _, leaked := range []string{"acme", "widget", "private-org"} {
				if strings.Contains(recorder.Body.String(), leaked) {
					t.Fatalf("scope response leaked %q: %s", leaked, recorder.Body.String())
				}
			}
		})
	}
}
