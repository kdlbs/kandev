package cursorcloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/secrets"
)

type configSecretStore struct {
	scope secrets.SecretScope
	value string
}

func (s configSecretStore) Create(context.Context, *secrets.SecretWithValue) error { return nil }
func (s configSecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if id != "key-ref" {
		return nil, secrets.ErrNotFound
	}
	return &secrets.Secret{ID: id, Scope: s.scope}, nil
}
func (s configSecretStore) Reveal(_ context.Context, id string) (string, error) {
	if id != "key-ref" {
		return "", secrets.ErrNotFound
	}
	return s.value, nil
}
func (configSecretStore) Update(context.Context, string, *secrets.UpdateSecretRequest) error {
	return nil
}
func (configSecretStore) Delete(context.Context, string) error                    { return nil }
func (configSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) { return nil, nil }
func (configSecretStore) Close() error                                            { return nil }

type configClientStub struct{}

func (configClientStub) ListModels(context.Context) (ModelCatalog, error) {
	return ModelCatalog{Items: []Model{{ID: "model-1", DisplayName: "Model 1"}}}, nil
}
func (configClientStub) ListRepositories(context.Context) (RepositoryCatalog, error) {
	return RepositoryCatalog{Items: []Repository{{URL: "https://github.com/owner/repo"}}}, nil
}

func TestCursorCloudConfigHandlersRequireGlobalSecretAndHideProviderErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	factoryCalls := 0
	factory := func(apiKey string) (ConfigClient, error) {
		factoryCalls++
		if apiKey != "test-api-key" {
			t.Fatalf("factory API key = %q", apiKey)
		}
		return configClientStub{}, nil
	}
	router := gin.New()
	RegisterConfigRoutes(router, configSecretStore{scope: secrets.ScopeGlobal, value: "test-api-key"}, true, factory)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cursor-cloud/catalog", strings.NewReader(
		`{"secret_id":"key-ref","callback_url":"https://callback.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"model-1"`) ||
		!strings.Contains(response.Body.String(), `"https://github.com/owner/repo"`) {
		t.Fatalf("catalog response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "test-api-key") {
		t.Fatal("catalog response exposed the API key")
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cursor-cloud/test", strings.NewReader(
		`{"secret_id":"key-ref","callback_url":"https://callback.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"callback_route":"configured"`) ||
		!strings.Contains(response.Body.String(), `"cursor_reachability":"not_verified"`) {
		t.Fatalf("connection diagnostics = %d %s", response.Code, response.Body.String())
	}

	router = gin.New()
	RegisterConfigRoutes(router, configSecretStore{scope: secrets.ScopeWorkspace, value: "test-api-key"}, true, factory)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cursor-cloud/test", strings.NewReader(
		`{"secret_id":"key-ref","callback_url":"https://callback.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || factoryCalls != 2 {
		t.Fatalf("workspace secret response = %d %s, factory calls %d", response.Code, response.Body.String(), factoryCalls)
	}
	if strings.Contains(response.Body.String(), "test-api-key") {
		t.Fatal("secret validation error exposed the API key")
	}
}

func TestCursorCloudConfigRoutesStayAbsentWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	factoryCalls := 0
	router := gin.New()
	RegisterConfigRoutes(router, configSecretStore{scope: secrets.ScopeGlobal, value: "test-api-key"}, false,
		func(string) (ConfigClient, error) {
			factoryCalls++
			return configClientStub{}, nil
		})
	if routes := router.Routes(); len(routes) != 0 {
		t.Fatalf("registered routes while disabled = %+v, want none", routes)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cursor-cloud/catalog", strings.NewReader(`{"secret_id":"key-ref","callback_url":"https://callback.example.test"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound || factoryCalls != 0 {
		t.Fatalf("disabled route response = %d %s, factory calls %d", response.Code, response.Body.String(), factoryCalls)
	}
}

type failingConfigClient struct{}

func (failingConfigClient) ListModels(context.Context) (ModelCatalog, error) {
	return ModelCatalog{}, errors.New("provider error includes secret test-api-key")
}
func (failingConfigClient) ListRepositories(context.Context) (RepositoryCatalog, error) {
	return RepositoryCatalog{}, errors.New("provider error includes secret test-api-key")
}

func TestCursorCloudConfigHandlersSanitizeProviderFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterConfigRoutes(router, configSecretStore{scope: secrets.ScopeGlobal, value: "test-api-key"}, true,
		func(string) (ConfigClient, error) { return failingConfigClient{}, nil })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cursor-cloud/test", strings.NewReader(
		`{"secret_id":"key-ref","callback_url":"https://callback.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "test-api-key") {
		t.Fatalf("provider error response = %d %s", response.Code, response.Body.String())
	}
}
