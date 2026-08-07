package httpmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newTestService builds an auth service with the features.auth flag set to
// authEnabled (on ⇒ setup mode until an admin is created via setupAdmin).
func newTestService(t *testing.T, authEnabled bool) *auth.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	store, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = authEnabled
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{
		Cfg: cfg, Store: store, Users: users,
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	return svc
}

// newTestRouter returns a router with the auth middleware and one probe route
// that echoes the resolved identity. Requests to unregistered paths report
// 404 when the middleware passed them through, 401 when it aborted.
func newTestRouter(svc *auth.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(svc))
	router.GET("/api/v1/probe", func(c *gin.Context) {
		identity, ok := authn.FromGin(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no identity"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": identity.UserID, "synthetic": identity.Synthetic})
	})
	return router
}

func doRequest(router *gin.Engine, method, path string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for _, m := range mutate {
		m(req)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// setupAdmin completes the setup wizard on a flag-on (setup-mode) service and
// returns the admin's session-cookie token.
func setupAdmin(t *testing.T, svc *auth.Service) (cookieToken string) {
	t.Helper()
	_, token, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestDisabledModeInjectsSyntheticAdmin(t *testing.T) {
	svc := newTestService(t, false)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, userstore.DefaultUserID) || !contains(body, `"synthetic":true`) {
		t.Fatalf("expected synthetic default-user identity, got %s", body)
	}
}

func TestEnabledModeBlocksAPIWithoutCredentials(t *testing.T) {
	svc := newTestService(t, true)
	setupAdmin(t, svc)
	router := newTestRouter(svc)

	if rec := doRequest(router, http.MethodGet, "/api/v1/probe"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("probe without credentials: %d", rec.Code)
	}
}

func TestEnabledModeSessionCookieAuthenticates(t *testing.T) {
	svc := newTestService(t, true)
	token := setupAdmin(t, svc)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: svc.CookieName(), Value: token})
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("probe with session: %d body=%s", rec.Code, rec.Body.String())
	}
	if contains(rec.Body.String(), `"synthetic":true`) {
		t.Fatal("real session must not be synthetic")
	}
}

func TestEnabledModePATAuthenticates(t *testing.T) {
	svc := newTestService(t, true)
	setupAdmin(t, svc)
	_, pat, err := svc.MintToken(context.Background(), userstore.DefaultUserID, "ci", 0)
	if err != nil {
		t.Fatal(err)
	}
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+pat)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("probe with PAT: %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestEnabledModeAllowlistMatrix pins the pass/deny policy for every path
// class from the plan's allowlist spec. 404 = middleware passed through
// (route unregistered), 401 = middleware denied.
func TestEnabledModeAllowlistMatrix(t *testing.T) {
	svc := newTestService(t, true)
	setupAdmin(t, svc)
	router := newTestRouter(svc)

	cases := []struct {
		name    string
		method  string
		path    string
		mutate  []func(*http.Request)
		blocked bool
	}{
		{name: "health", method: http.MethodGet, path: "/health"},
		{name: "features", method: http.MethodGet, path: "/api/v1/features"},
		{name: "app-state", method: http.MethodGet, path: "/api/v1/app-state"},
		{name: "login", method: http.MethodPost, path: "/api/v1/auth/login"},
		{name: "setup", method: http.MethodPost, path: "/api/v1/auth/setup"},
		{name: "invite accept", method: http.MethodPost, path: "/api/v1/auth/invites/accept"},
		{name: "auth me", method: http.MethodGet, path: "/api/v1/auth/me"},
		{name: "automation webhook", method: http.MethodPost, path: "/api/v1/automations/webhook/abc"},
		{name: "office channel inbound", method: http.MethodPost, path: "/api/v1/office/channels/ch1/inbound"},
		{name: "plugin webhook", method: http.MethodPost, path: "/api/plugins/p1/webhooks/key1"},
		{name: "ws upgrade deferred", method: http.MethodGet, path: "/ws"},
		{name: "terminal deferred", method: http.MethodGet, path: "/terminal/target"},
		{name: "vscode proxy deferred", method: http.MethodGet, path: "/vscode/s1/index.html"},
		{name: "port proxy deferred", method: http.MethodGet, path: "/port-proxy/s1/3000"},
		{name: "mcp deferred", method: http.MethodGet, path: "/mcp"},
		{name: "spa shell", method: http.MethodGet, path: "/settings/system"},
		{name: "static asset", method: http.MethodGet, path: "/assets/app.js"},
		{
			name: "office with bearer deferred", method: http.MethodGet, path: "/api/v1/office/tasks/t1",
			mutate: []func(*http.Request){func(r *http.Request) { r.Header.Set("Authorization", "Bearer agent.jwt.here") }},
		},

		{name: "github credential broker readiness", method: http.MethodGet, path: "/api/v1/github/credentials/resolve"},
		{name: "github credential broker resolve", method: http.MethodPost, path: "/api/v1/github/credentials/resolve"},
		{
			name: "github app webhook", method: http.MethodPost,
			path: "/api/v1/github/app/registrations/reg1/webhook",
		},

		{name: "office without bearer", method: http.MethodGet, path: "/api/v1/office/tasks/t1", blocked: true},
		{name: "tasks api", method: http.MethodGet, path: "/api/v1/tasks", blocked: true},
		{name: "workspaces api", method: http.MethodGet, path: "/api/v1/workspaces", blocked: true},
		{name: "plugin management", method: http.MethodGet, path: "/api/plugins", blocked: true},
		{name: "plugin bundle unauthenticated", method: http.MethodGet, path: "/api/plugins/p1/bundle", blocked: true},
		{
			name: "plugin user-state unauthenticated", method: http.MethodGet,
			path: "/api/plugins/p1/user-state/task/task1/note", blocked: true,
		},
		{name: "debug", method: http.MethodGet, path: "/debug/vars", blocked: true},
		{
			name: "github connections requires auth", method: http.MethodGet,
			path: "/api/v1/github/connections", blocked: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(router, tc.method, tc.path, tc.mutate...)
			if tc.blocked && rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
			}
			if !tc.blocked && rec.Code == http.StatusUnauthorized {
				t.Fatalf("%s %s = 401, want pass-through", tc.method, tc.path)
			}
		})
	}
}

func TestSetupModeAllowsOnlyBootstrapSurfaces(t *testing.T) {
	// Flag on but no admin yet ⇒ setup mode.
	svc := newTestService(t, true)
	if svc.Mode() != auth.ModeSetup {
		t.Fatalf("mode = %s, want setup", svc.Mode())
	}
	router := newTestRouter(svc)

	if rec := doRequest(router, http.MethodPost, "/api/v1/auth/setup"); rec.Code == http.StatusUnauthorized {
		t.Fatal("setup endpoint must be reachable in setup mode")
	}
	if rec := doRequest(router, http.MethodGet, "/api/v1/probe"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("api must be blocked in setup mode, got %d", rec.Code)
	}
	if rec := doRequest(router, http.MethodGet, "/"); rec.Code == http.StatusUnauthorized {
		t.Fatal("shell must render in setup mode")
	}
}

func TestInvalidCredentialsAreRejected(t *testing.T) {
	svc := newTestService(t, true)
	setupAdmin(t, svc)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: svc.CookieName(), Value: "forged-token"})
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie: %d", rec.Code)
	}
	rec = doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer kandev_pat_deadbeef_forged")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged PAT: %d", rec.Code)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
