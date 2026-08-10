package forgejo

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type fakeWorkspaceSecretStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeWorkspaceSecretStore() *fakeWorkspaceSecretStore {
	return &fakeWorkspaceSecretStore{values: make(map[string]string)}
}

func (s *fakeWorkspaceSecretStore) Reveal(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (s *fakeWorkspaceSecretStore) Set(_ context.Context, key, _ string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *fakeWorkspaceSecretStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func (s *fakeWorkspaceSecretStore) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.values[key]
	return ok, nil
}

func newConfigTestStore(t *testing.T) *Store {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func newConfigTestService(t *testing.T) (*Service, *fakeWorkspaceSecretStore) {
	t.Helper()
	secrets := newFakeWorkspaceSecretStore()
	return NewService(newConfigTestStore(t), secrets), secrets
}

func TestStore_ConfigIsScopedToWorkspace(t *testing.T) {
	store := newConfigTestStore(t)
	ctx := context.Background()
	if err := store.SaveConfig(ctx, &Config{WorkspaceID: "workspace-a", Origin: "https://forgejo.example", Username: "alice", LastOK: true}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	got, err := store.GetConfig(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("get saved config: %v", err)
	}
	if got == nil || got.Username != "alice" || got.Revision != 1 {
		t.Fatalf("unexpected saved config: %+v", got)
	}
	missing, err := store.GetConfig(ctx, "workspace-b")
	if err != nil {
		t.Fatalf("get missing config: %v", err)
	}
	if missing != nil {
		t.Fatalf("workspace-b config = %+v, want nil", missing)
	}
}

func TestService_SetConfigTestsBeforePersisting(t *testing.T) {
	service, secrets := newConfigTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"login":"alice"}`))
	}))
	t.Cleanup(server.Close)

	config, err := service.SetConfig(context.Background(), "workspace-a", &SetConfigRequest{Origin: server.URL + "/", Token: " test-token "})
	if err != nil {
		t.Fatalf("set config: %v", err)
	}
	if config.Origin != server.URL || !config.HasSecret || config.Username != "alice" || !config.LastOK {
		t.Fatalf("unexpected config: %+v", config)
	}
	if got, err := secrets.Reveal(context.Background(), SecretKeyForWorkspace("workspace-a")); err != nil || got != "test-token" {
		t.Fatalf("stored secret = %q, %v", got, err)
	}
}

func TestService_SetConfigFailureDoesNotPersist(t *testing.T) {
	service, _ := newConfigTestService(t)
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	if _, err := service.SetConfig(context.Background(), "workspace-a", &SetConfigRequest{Origin: server.URL, Token: "token"}); err == nil {
		t.Fatal("SetConfig succeeded with an unauthenticated Forgejo endpoint")
	}
	config, err := service.GetConfig(context.Background(), "workspace-a")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if config != nil {
		t.Fatalf("config persisted after failed connection test: %+v", config)
	}
}

func TestService_SetConfigRetainsExistingToken(t *testing.T) {
	service, secrets := newConfigTestService(t)
	if err := secrets.Set(context.Background(), SecretKeyForWorkspace("workspace-a"), "Forgejo token", "existing-token"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token existing-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"login":"alice"}`))
	}))
	t.Cleanup(server.Close)

	if _, err := service.SetConfig(context.Background(), "workspace-a", &SetConfigRequest{Origin: server.URL}); err != nil {
		t.Fatalf("update config while retaining token: %v", err)
	}
}

func TestController_RequiresWorkspaceAndReportsUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newConfigTestService(t)
	router := gin.New()
	RegisterRoutes(router, service)

	missingWorkspace := httptest.NewRecorder()
	router.ServeHTTP(missingWorkspace, httptest.NewRequest(http.MethodGet, "/api/v1/forgejo/config", nil))
	if missingWorkspace.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d, want %d", missingWorkspace.Code, http.StatusBadRequest)
	}

	unconfigured := httptest.NewRecorder()
	router.ServeHTTP(unconfigured, httptest.NewRequest(http.MethodGet, "/api/v1/forgejo/config?workspace_id=workspace-a", nil))
	if unconfigured.Code != http.StatusNoContent {
		t.Fatalf("unconfigured status = %d, want %d", unconfigured.Code, http.StatusNoContent)
	}
}

func TestService_ClientForWorkspaceUsesWorkspaceSecret(t *testing.T) {
	service, secrets := newConfigTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token workspace-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)
	if err := service.store.SaveConfig(context.Background(), &Config{WorkspaceID: "workspace-a", Origin: server.URL}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := secrets.Set(context.Background(), SecretKeyForWorkspace("workspace-a"), "Forgejo token", "workspace-token"); err != nil {
		t.Fatalf("set token: %v", err)
	}

	repositories, _, err := service.ListRepositories(context.Background(), "workspace-a", 1, 30)
	if err != nil || len(repositories) != 0 {
		t.Fatalf("list repositories = %#v, %v", repositories, err)
	}
}

func TestService_ListsWorkspaceQueue(t *testing.T) {
	service, secrets := newConfigTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/repos":
			_, _ = w.Write([]byte(`[{"name":"repo","full_name":"owner/repo","owner":{"login":"owner"}}]`))
		case "/api/v1/repos/owner/repo/issues":
			_, _ = w.Write([]byte(`[{"number":1,"title":"Issue","state":"open"}]`))
		case "/api/v1/repos/owner/repo/pulls":
			_, _ = w.Write([]byte(`[{"number":2,"title":"PR","state":"open","head":{"ref":"feature"},"base":{"ref":"main"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	if err := service.store.SaveConfig(context.Background(), &Config{WorkspaceID: "workspace-a", Origin: server.URL}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Set(context.Background(), SecretKeyForWorkspace("workspace-a"), "Forgejo token", "token"); err != nil {
		t.Fatal(err)
	}
	issues, pulls, err := service.ListWorkspaceQueue(context.Background(), "workspace-a")
	if err != nil || len(issues) != 1 || len(pulls) != 1 || pulls[0].PullRequest.Number != 2 {
		t.Fatalf("issues=%#v pulls=%#v err=%v", issues, pulls, err)
	}
}
