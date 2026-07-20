package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func TestHTTPCreateRepositoryRejectsInvalidLocalPathWithoutPersistence(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)
	body, err := json.Marshal(httpCreateRepositoryRequest{
		Name:       "Not Git",
		SourceType: "local",
		LocalPath:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/repositories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("invalid repository was persisted: %+v", repositories)
	}
}

type repositoryHandlerRemoteLister struct {
	calls int
}

func (l *repositoryHandlerRemoteLister) ListRepoBranches(_ context.Context, owner, name string) ([]service.Branch, error) {
	l.calls++
	if owner != "owner" || name != "repo" {
		return nil, errors.New("unexpected provider identity")
	}
	return []service.Branch{{Name: "main", Type: "remote"}}, nil
}

func TestHTTPListRepositoryBranchesUsesRepositoryIdentity(t *testing.T) {
	router, repo, svc := newRepositoryHTTPTestRouterWithService(t)
	if err := repo.CreateRepository(context.Background(), &models.Repository{
		ID:            "provider-repo",
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     t.TempDir(),
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	svc.SetRemoteBranchLister(&repositoryHandlerRemoteLister{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/provider-repo/branches", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"main"`) {
		t.Fatalf("response missing remote main branch: %s", response.Body.String())
	}
}

func TestHTTPListBranchesRejectsRepositoryFromAnotherWorkspace(t *testing.T) {
	router, repo, svc := newRepositoryHTTPTestRouterWithService(t)
	for _, workspaceID := range []string{"ws-a", "ws-b"} {
		if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
			t.Fatalf("CreateWorkspace %s: %v", workspaceID, err)
		}
	}
	if err := repo.CreateRepository(context.Background(), &models.Repository{
		ID:            "provider-repo",
		WorkspaceID:   "ws-b",
		Name:          "owner/repo",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	lister := &repositoryHandlerRemoteLister{}
	svc.SetRemoteBranchLister(lister)

	crossWorkspace := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/ws-a/branches?repository_id=provider-repo",
		nil,
	)
	crossResponse := httptest.NewRecorder()
	router.ServeHTTP(crossResponse, crossWorkspace)
	if crossResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace status = %d, want %d; body = %s", crossResponse.Code, http.StatusNotFound, crossResponse.Body.String())
	}
	if lister.calls != 0 {
		t.Fatalf("provider lister calls after rejected cross-workspace request = %d, want 0", lister.calls)
	}

	sameWorkspace := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/ws-b/branches?repository_id=provider-repo",
		nil,
	)
	sameResponse := httptest.NewRecorder()
	router.ServeHTTP(sameResponse, sameWorkspace)
	if sameResponse.Code != http.StatusOK {
		t.Fatalf("same-workspace status = %d, want %d; body = %s", sameResponse.Code, http.StatusOK, sameResponse.Body.String())
	}
	if lister.calls != 1 {
		t.Fatalf("provider lister calls = %d, want 1", lister.calls)
	}
}

func TestHTTPListBranchesRejectsInvalidExplicitPath(t *testing.T) {
	router, _ := newRepositoryHTTPTestRouter(t)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/ws-1/branches?path="+url.QueryEscape(t.TempDir()),
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func newRepositoryHTTPTestRouter(t *testing.T) (*gin.Engine, *taskrepo.Repository) {
	t.Helper()
	router, repo, _ := newRepositoryHTTPTestRouterWithService(t)
	return router, repo
}

func newRepositoryHTTPTestRouterWithService(t *testing.T) (*gin.Engine, *taskrepo.Repository, *service.Service) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repository.Provide: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup repository: %v", err)
		}
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	svc := service.NewService(service.Repos{
		Workspaces:   repo,
		RepoEntities: repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	NewRepositoryHandlers(svc, log).registerHTTP(router)
	return router, repo, svc
}

// TestRepositoryCreateRequestJSONIncludesCopyFiles verifies that the
// copy_files field is wired through the JSON encoding/decoding for both the
// HTTP and WS create-repository request shapes. Failure here means the
// handler will silently drop a `copy_files` value sent by the client.
func TestRepositoryCreateRequestJSONIncludesCopyFiles(t *testing.T) {
	t.Run("http_marshal_contains_copy_files", func(t *testing.T) {
		req := httpCreateRepositoryRequest{Name: "r", CopyFiles: ".env"}
		b, err := json.Marshal(&req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(b), `"copy_files":".env"`) {
			t.Errorf("missing copy_files in JSON: %s", b)
		}
	})

	t.Run("http_unmarshal_populates_copy_files", func(t *testing.T) {
		var req httpCreateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"name":"r","copy_files":".env, *.local"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != ".env, *.local" {
			t.Errorf("CopyFiles = %q, want %q", req.CopyFiles, ".env, *.local")
		}
	})

	t.Run("ws_unmarshal_populates_copy_files", func(t *testing.T) {
		var req wsCreateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"workspace_id":"w","name":"r","copy_files":".env"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != ".env" {
			t.Errorf("CopyFiles = %q, want %q", req.CopyFiles, ".env")
		}
	})
}

// TestRepositoryUpdateRequestJSONCopyFilesPointer verifies the pointer-style
// copy_files field on update requests round-trips correctly — nil when
// omitted, non-nil and dereferenceable to the supplied value when present.
func TestRepositoryUpdateRequestJSONCopyFilesPointer(t *testing.T) {
	t.Run("http_unmarshal_sets_pointer", func(t *testing.T) {
		var req httpUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"copy_files":".env, *.local"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles == nil {
			t.Fatal("CopyFiles is nil, want non-nil")
		}
		if *req.CopyFiles != ".env, *.local" {
			t.Errorf("*CopyFiles = %q, want %q", *req.CopyFiles, ".env, *.local")
		}
	})

	t.Run("http_omitted_leaves_nil", func(t *testing.T) {
		var req httpUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"name":"r"}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles != nil {
			t.Errorf("CopyFiles = %v, want nil", req.CopyFiles)
		}
	})

	t.Run("ws_unmarshal_sets_pointer", func(t *testing.T) {
		var req wsUpdateRepositoryRequest
		if err := json.Unmarshal([]byte(`{"id":"x","copy_files":""}`), &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.CopyFiles == nil {
			t.Fatal("CopyFiles is nil, want non-nil pointer to empty string")
		}
		if *req.CopyFiles != "" {
			t.Errorf("*CopyFiles = %q, want empty string", *req.CopyFiles)
		}
	})
}
