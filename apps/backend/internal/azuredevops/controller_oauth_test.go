package azuredevops

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func newOAuthRouteFixture(t *testing.T) (*gin.Engine, *oauthTestHarness) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newOAuthTestHarness(t)
	router := gin.New()
	RegisterRoutes(router, h.svc, logger.Default())
	return router, h
}

func TestOAuthAvailabilityRoute(t *testing.T) {
	router, h := newOAuthRouteFixture(t)
	type availability struct {
		Available          bool   `json:"available"`
		DefaultClientID    string `json:"defaultClientId"`
		DefaultRedirectURL string `json:"defaultRedirectUrl"`
	}
	available := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/oauth/availability?workspace_id=ws-1", nil)
	got := decodeJSON[availability](t, available)
	if available.Code != http.StatusOK || !got.Available || got.DefaultClientID != "client-1" ||
		got.DefaultRedirectURL != testOAuthRedirectURL {
		t.Fatalf("availability = %d %s", available.Code, available.Body.String())
	}
	h.svc.ConfigureOAuth(OAuthSettings{RedirectURL: testOAuthRedirectURL})
	unavailable := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/oauth/availability?workspace_id=ws-1", nil)
	if got := decodeJSON[availability](t, unavailable); got.Available || got.DefaultClientID != "" {
		t.Fatalf("availability without client ID = %s", unavailable.Body.String())
	}
	start := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/start?workspace_id=ws-1",
		map[string]any{"organizationUrl": "https://dev.azure.com/acme"})
	if start.Code != http.StatusConflict || decodeJSON[map[string]string](t, start)["code"] != "azure_devops_oauth_unavailable" {
		t.Fatalf("start without client ID = %d %s", start.Code, start.Body.String())
	}
}

func TestOAuthStartAndCompleteRoutes(t *testing.T) {
	router, _ := newOAuthRouteFixture(t)
	const query = "?workspace_id=ws-1"
	started := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/start"+query,
		map[string]any{"organizationUrl": "https://dev.azure.com/acme"})
	if started.Code != http.StatusOK {
		t.Fatalf("start = %d %s", started.Code, started.Body.String())
	}
	body := decodeJSON[StartOAuthResponse](t, started)
	if !strings.HasPrefix(body.AuthURL, "https://login.microsoftonline.com/tenant-1/") || body.State == "" {
		t.Fatalf("start response = %+v", body)
	}

	invalid := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/complete"+query,
		map[string]any{"code": "c", "state": "wrong"})
	if invalid.Code != http.StatusBadRequest || decodeJSON[map[string]string](t, invalid)["code"] != "azure_devops_oauth_invalid_state" {
		t.Fatalf("invalid complete = %d %s", invalid.Code, invalid.Body.String())
	}

	completed := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/complete"+query,
		map[string]any{"code": "c", "state": body.State})
	if completed.Code != http.StatusOK {
		t.Fatalf("complete = %d %s", completed.Code, completed.Body.String())
	}
	cfg := decodeJSON[Config](t, completed)
	if cfg.AuthMethod != AuthMethodOAuth || cfg.OAuthAccount != "Ana Silva" || !cfg.HasSecret {
		t.Fatalf("completed config = %+v", cfg)
	}
	if strings.Contains(completed.Body.String(), "access-1") || strings.Contains(completed.Body.String(), "refresh-1") {
		t.Fatalf("complete response leaks a token: %s", completed.Body.String())
	}
}

func TestOAuthCallbackPagePostsResultToOpenerOnly(t *testing.T) {
	router, _ := newOAuthRouteFixture(t)
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/azure-devops/oauth/callback?code=abc%3C%2Fscript%3E&state=s1", nil)
	request.Header.Set("Accept-Language", "pt-PT")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	page := response.Body.String()
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("callback = %d %s", response.Code, response.Header().Get("Content-Type"))
	}
	for _, want := range []string{`azure-devops-oauth-callback`, `window.opener.postMessage`, `window.location.origin`, `lang="pt-pt"`, "Volte ao Kandev"} {
		if !strings.Contains(page, want) {
			t.Errorf("callback page is missing %q", want)
		}
	}
	if strings.Contains(page, "abc</script>") || strings.Contains(page, `"*"`) {
		t.Fatalf("callback page must not echo query values or post to any origin: %s", page)
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback headers = %v", response.Header())
	}
}

func TestMockOAuthAuthorizeRedirectsToRelativeCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(mockEnvVar, "true")
	service, _, err := Provide(newTestDB(t), nil, newFakeSecretStore(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !service.OAuthAvailable() {
		t.Fatal("mock mode must make Microsoft sign-in available")
	}
	router := gin.New()
	RegisterRoutes(router, service, logger.Default())
	RegisterMockRoutes(router, service, logger.Default())
	started := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/start?workspace_id=ws-1",
		map[string]any{"organizationUrl": "https://dev.azure.com/acme"})
	body := decodeJSON[StartOAuthResponse](t, started)
	if !strings.HasPrefix(body.AuthURL, "/api/v1/azure-devops/mock/oauth/authorize?") {
		t.Fatalf("mock authorize URL = %q", body.AuthURL)
	}
	authorize := performRequest(t, router, http.MethodGet, body.AuthURL, nil)
	location, err := url.Parse(authorize.Header().Get("Location"))
	if authorize.Code != http.StatusFound || err != nil || location.Path != OAuthCallbackPath || location.Host != "" ||
		location.Query().Get("state") != body.State || location.Query().Get("code") == "" {
		t.Fatalf("mock authorize = %d %q", authorize.Code, authorize.Header().Get("Location"))
	}
	completed := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/complete?workspace_id=ws-1",
		map[string]any{"code": location.Query().Get("code"), "state": body.State})
	if completed.Code != http.StatusOK || decodeJSON[Config](t, completed).AuthMethod != AuthMethodOAuth {
		t.Fatalf("mock complete = %d %s", completed.Code, completed.Body.String())
	}
}
