package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func performRegisterRemoteRepositoryRequest(
	t *testing.T,
	router *gin.Engine,
	workspaceID string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/repositories/remote",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestHTTPRegisterRemoteRepositoryCreatesGitHubRepository(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)

	response := performRegisterRemoteRepositoryRequest(t, router, "ws-1",
		`{"remote_url":"https://github.com/acme/api","default_branch":"develop"}`)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var created struct {
		ID            string `json:"id"`
		WorkspaceID   string `json:"workspace_id"`
		Provider      string `json:"provider"`
		ProviderOwner string `json:"provider_owner"`
		ProviderName  string `json:"provider_name"`
		RemoteURL     string `json:"remote_url"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if created.WorkspaceID != "ws-1" || created.Provider != "github" || created.ProviderOwner != "acme" ||
		created.ProviderName != "api" || created.RemoteURL != "https://github.com/acme/api.git" ||
		created.DefaultBranch != "develop" {
		t.Fatalf("created repository = %+v, want GitHub acme/api on develop in ws-1", created)
	}
	stored, err := repo.GetRepository(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.RemoteURL != "https://github.com/acme/api.git" {
		t.Fatalf("stored RemoteURL = %q, want canonical clone URL", stored.RemoteURL)
	}
}

func TestHTTPRegisterRemoteRepositoryReturnsOKForExistingRepository(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)
	body := `{"remote_url":"https://github.com/acme/api"}`

	first := performRegisterRemoteRepositoryRequest(t, router, "ws-1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d; body = %s", first.Code, http.StatusCreated, first.Body.String())
	}
	second := performRegisterRemoteRepositoryRequest(t, router, "ws-1", body)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d; body = %s", second.Code, http.StatusOK, second.Body.String())
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories = %d, want exactly one", len(repositories))
	}
}

func TestHTTPRegisterRemoteRepositoryMapsClientErrors(t *testing.T) {
	t.Run("missing remote_url", func(t *testing.T) {
		router, _ := newRepositoryHTTPTestRouter(t)
		response := performRegisterRemoteRepositoryRequest(t, router, "ws-1", `{"provider":"github"}`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})

	t.Run("unsupported host without provider", func(t *testing.T) {
		router, repo := newRepositoryHTTPTestRouter(t)
		response := performRegisterRemoteRepositoryRequest(t, router, "ws-1",
			`{"remote_url":"https://git.example.test/acme/api.git"}`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"error_code":"repository_selection_invalid"`) {
			t.Fatalf("body = %s, want repository_selection_invalid error code", response.Body.String())
		}
		repositories, err := repo.ListRepositories(context.Background(), "ws-1")
		if err != nil || len(repositories) != 0 {
			t.Fatalf("repositories = %+v, error %v; want none", repositories, err)
		}
	})

	t.Run("provider hint mismatch", func(t *testing.T) {
		router, repo := newRepositoryHTTPTestRouter(t)
		response := performRegisterRemoteRepositoryRequest(t, router, "ws-1",
			`{"remote_url":"https://github.com/acme/api","provider_owner":"someone-else"}`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"error_code":"repository_selection_invalid"`) {
			t.Fatalf("body = %s, want repository_selection_invalid error code", response.Body.String())
		}
		repositories, err := repo.ListRepositories(context.Background(), "ws-1")
		if err != nil || len(repositories) != 0 {
			t.Fatalf("repositories = %+v, error %v; want none", repositories, err)
		}
	})

	t.Run("unknown workspace", func(t *testing.T) {
		router, _ := newRepositoryHTTPTestRouter(t)
		response := performRegisterRemoteRepositoryRequest(t, router, "missing",
			`{"remote_url":"https://github.com/acme/api"}`)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
		}
	})
}
