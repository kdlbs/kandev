package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

func TestHTTPInitializeLocalRepositoryCreatesRepository(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)
	parentPath := t.TempDir()
	body := []byte(`{"name":"new-project","parent_path":` + strconv.Quote(parentPath) + `}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/ws-1/repositories/initialize-local",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var created struct {
		ID            string `json:"id"`
		WorkspaceID   string `json:"workspace_id"`
		Name          string `json:"name"`
		SourceType    string `json:"source_type"`
		LocalPath     string `json:"local_path"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	wantPath := filepath.Join(parentPath, "new-project")
	if created.WorkspaceID != "ws-1" || created.Name != "new-project" || created.SourceType != "local" ||
		created.LocalPath != wantPath || created.DefaultBranch != "main" {
		t.Fatalf("created repository = %+v, want workspace ws-1 local repository at %q on main", created, wantPath)
	}
	stored, err := repo.GetRepository(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != wantPath || stored.DefaultBranch != "main" {
		t.Fatalf("stored repository = %+v, want path %q on main", stored, wantPath)
	}
}

func TestHTTPInitializeLocalRepositoryMapsClientErrors(t *testing.T) {
	t.Run("invalid input", func(t *testing.T) {
		router, _ := newRepositoryHTTPTestRouter(t)
		response := performInitializeLocalRepositoryRequest(t, router, "ws-1", `{"name":"nested/name","parent_path":"/tmp"}`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})

	t.Run("unknown workspace before mutation", func(t *testing.T) {
		router, _ := newRepositoryHTTPTestRouter(t)
		parent := t.TempDir()
		body := `{"name":"new-project","parent_path":` + strconv.Quote(parent) + `}`
		response := performInitializeLocalRepositoryRequest(t, router, "missing", body)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
		}
		if _, err := os.Stat(filepath.Join(parent, "new-project")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target stat error = %v, want not exist", err)
		}
	})

	t.Run("existing target", func(t *testing.T) {
		router, repo := newRepositoryHTTPTestRouter(t)
		parent := t.TempDir()
		target := filepath.Join(parent, "existing")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("Mkdir target: %v", err)
		}
		marker := filepath.Join(target, "marker")
		if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
			t.Fatalf("WriteFile marker: %v", err)
		}
		body := `{"name":"existing","parent_path":` + strconv.Quote(parent) + `}`
		response := performInitializeLocalRepositoryRequest(t, router, "ws-1", body)
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusConflict, response.Body.String())
		}
		content, err := os.ReadFile(marker)
		if err != nil || string(content) != "keep" {
			t.Fatalf("marker = %q, error %v; existing target was modified", content, err)
		}
		repositories, err := repo.ListRepositories(context.Background(), "ws-1")
		if err != nil || len(repositories) != 0 {
			t.Fatalf("repositories = %+v, error %v; want none", repositories, err)
		}
	})
}

func performInitializeLocalRepositoryRequest(
	t *testing.T,
	router *gin.Engine,
	workspaceID string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/repositories/initialize-local",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestHTTPCreateRepositoryIgnoresRemoteURL(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/ws-1/repositories",
		strings.NewReader(`{"name":"owner/repo","source_type":"provider","remote_url":"https://github.com/owner/repo.git"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories = %d, want 1", len(repositories))
	}
	if repositories[0].RemoteURL != "" {
		t.Errorf("RemoteURL = %q, want empty; generic repository creation must not accept clone URLs", repositories[0].RemoteURL)
	}
}

func TestHTTPUpdateRepositoryIgnoresRemoteURL(t *testing.T) {
	router, repo := newRepositoryHTTPTestRouter(t)
	if err := repo.CreateRepository(context.Background(), &models.Repository{
		ID:          "provider-repo",
		WorkspaceID: "ws-1",
		Name:        "owner/repo",
		SourceType:  "provider",
		RemoteURL:   "https://github.com/owner/old.git",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/repositories/provider-repo",
		strings.NewReader(`{"remote_url":"https://github.com/owner/repo.git"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	repository, err := repo.GetRepository(context.Background(), "provider-repo")
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repository.RemoteURL != "https://github.com/owner/old.git" {
		t.Errorf("RemoteURL = %q, want original value %q", repository.RemoteURL, "https://github.com/owner/old.git")
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

func TestHTTPListDirectoryIncludesChoosableContract(t *testing.T) {
	router, _ := newRepositoryHTTPTestRouter(t)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/fs/list-dir?path="+url.QueryEscape(t.TempDir()),
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Choosable *bool `json:"choosable"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Choosable == nil || !*body.Choosable {
		t.Fatalf("choosable = %v, want true; body = %s", body.Choosable, response.Body.String())
	}
}

func TestHTTPCreateDirectoryCreatesFolder(t *testing.T) {
	router, _ := newRepositoryHTTPTestRouter(t)
	parent := t.TempDir()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/fs/create-dir",
		strings.NewReader(`{"parent_path":`+strconv.Quote(parent)+`,"name":"projects"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	wantPath := filepath.Join(parent, "projects")
	if info, err := os.Stat(wantPath); err != nil || !info.IsDir() {
		t.Fatalf("created directory %q: info=%v error=%v", wantPath, info, err)
	}
	if !strings.Contains(response.Body.String(), strconv.Quote(wantPath)) {
		t.Fatalf("response missing created path %q: %s", wantPath, response.Body.String())
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

func TestHTTPLocalRepositoryStatusRejectsInvalidExplicitPath(t *testing.T) {
	router, _ := newRepositoryHTTPTestRouter(t)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/ws-1/repositories/local-status?path="+url.QueryEscape(t.TempDir()),
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
