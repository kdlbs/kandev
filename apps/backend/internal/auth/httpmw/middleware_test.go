package httpmw

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authhttpapi "github.com/kandev/kandev/internal/auth/httpapi"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newTestService builds an auth service with the features.auth flag set to
// authEnabled (on ⇒ setup mode until an admin is created via setupAdmin) and
// returns the backing auth store for direct session seeding.
func newTestService(t *testing.T, authEnabled bool) (*auth.Service, *authstore.Store) {
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
	return svc, store
}

// newTestRouter returns a router with the auth middleware and one probe route
// that echoes the resolved identity. Requests to unregistered paths report
// 404 when the middleware passed them through, 401 when it aborted. Trusted
// proxies mirror production: no trusted-proxy args clears gin's trust-all
// default exactly like configureTrustedProxies does.
func newTestRouter(svc *auth.Service, trustedProxies ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if len(trustedProxies) == 0 {
		trustedProxies = nil
	}
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		panic(err)
	}
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
	svc, _ := newTestService(t, false)
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
	svc, _ := newTestService(t, true)
	setupAdmin(t, svc)
	router := newTestRouter(svc)

	if rec := doRequest(router, http.MethodGet, "/api/v1/probe"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("probe without credentials: %d", rec.Code)
	}
}

func TestEnabledModeSessionCookieAuthenticates(t *testing.T) {
	svc, _ := newTestService(t, true)
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
	svc, _ := newTestService(t, true)
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

func TestEnabledModePATAuthenticatesPluginWebhook(t *testing.T) {
	svc, _ := newTestService(t, true)
	setupAdmin(t, svc)
	_, pat, err := svc.MintToken(context.Background(), userstore.DefaultUserID, "plugin-ui", 0)
	if err != nil {
		t.Fatal(err)
	}
	router := newTestRouter(svc)
	router.POST("/api/plugins/:id/webhooks/:key", func(c *gin.Context) {
		if _, ok := authn.FromGin(c); !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no identity"})
			return
		}
		c.Status(http.StatusOK)
	})

	rec := doRequest(router, http.MethodPost, "/api/plugins/p1/webhooks/transcribe", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+pat)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated plugin webhook: %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestEnabledModeAllowlistMatrix pins the pass/deny policy for every path
// class from the plan's allowlist spec. 404 = middleware passed through
// (route unregistered), 401 = middleware denied.
func TestEnabledModeAllowlistMatrix(t *testing.T) {
	svc, _ := newTestService(t, true)
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
		{name: "ready", method: http.MethodGet, path: "/ready"},
		{name: "features", method: http.MethodGet, path: "/api/v1/features"},
		{name: "app-state", method: http.MethodGet, path: "/api/v1/app-state"},
		{name: "login", method: http.MethodPost, path: "/api/v1/auth/login"},
		{name: "setup", method: http.MethodPost, path: "/api/v1/auth/setup"},
		{name: "invite accept", method: http.MethodPost, path: "/api/v1/auth/invites/accept"},
		{name: "auth me", method: http.MethodGet, path: "/api/v1/auth/me"},
		{name: "automation webhook", method: http.MethodPost, path: "/api/v1/automations/webhook/abc"},
		{name: "office channel inbound", method: http.MethodPost, path: "/api/v1/office/channels/ch1/inbound"},
		{
			// Deferred, not public: the middleware lets GET/POST
			// /api/plugins/<id>/webhooks/<key> requests through structurally
			// (isPluginWebhookPath) because it cannot read the plugin manifest to know
			// whether this specific webhook is public. This row is therefore a WEAKENED
			// pin: it only proves the path still passes the middleware, not that
			// anonymous callers reach the subprocess. The real 401-vs-relay policy is
			// enforced and tested in internal/plugins: see handlers_webhook_auth_test.go.
			name: "plugin webhook (deferred, not a policy pin)", method: http.MethodPost, path: "/api/plugins/p1/webhooks/key1",
		},
		{
			name: "plugin webhook GET (deferred, not a policy pin)", method: http.MethodGet,
			path: "/api/plugins/p1/webhooks/key1",
		},
		{
			// Unsupported methods are not registered webhook relay routes, so they should
			// not bypass the global auth challenge just because the path has the relay
			// shape.
			name: "plugin webhook unsupported method", method: http.MethodPut,
			path: "/api/plugins/p1/webhooks/key1", blocked: true,
		},
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
		{name: "github credential broker reissue", method: http.MethodPost, path: "/api/v1/github/credentials/reissue"},
		{
			name: "github app webhook", method: http.MethodPost,
			path: "/api/v1/github/app/registrations/reg1/webhook",
		},
		{
			name: "github app manifest callback", method: http.MethodGet,
			path: "/api/v1/github/app/registrations/reg1/manifest/callback",
		},
		{
			name: "github app installation callback", method: http.MethodGet,
			path: "/api/v1/github/app/registrations/reg1/install/callback",
		},
		{
			name: "github app personal callback", method: http.MethodGet,
			path: "/api/v1/github/app/registrations/reg1/personal/callback",
		},
		{
			name: "github app callback wrong method", method: http.MethodPost,
			path: "/api/v1/github/app/registrations/reg1/manifest/callback", blocked: true,
		},

		{name: "office without bearer", method: http.MethodGet, path: "/api/v1/office/tasks/t1", blocked: true},
		{
			// AC-14 (docs/specs/platform/requirements/health-endpoint-version.md): /health
			// gained a version field, but /api/v1/system/info is not on the
			// allowlist and must stay rejected — the change must not widen it.
			name: "system info", method: http.MethodGet, path: "/api/v1/system/info", blocked: true,
		},
		{name: "tasks api", method: http.MethodGet, path: "/api/v1/tasks", blocked: true},
		{name: "workspaces api", method: http.MethodGet, path: "/api/v1/workspaces", blocked: true},
		{name: "plugin management", method: http.MethodGet, path: "/api/plugins", blocked: true},
		{name: "plugin bundle unauthenticated", method: http.MethodGet, path: "/api/plugins/p1/bundle", blocked: true},
		{
			name: "plugin user-state unauthenticated", method: http.MethodGet,
			path: "/api/plugins/p1/user-state/task/task1/note", blocked: true,
		},
		{
			// AC6: the old strings.Contains(path, "/webhooks/") match let this
			// through as a "plugin webhook"; the structural isPluginWebhookPath
			// match (exactly ["", "api", "plugins", id, "webhooks", key]) rejects
			// it as unrelated to the webhook relay.
			name: "plugin user-state path containing webhooks substring", method: http.MethodGet,
			path: "/api/plugins/p1/user-state/task/t1/note/webhooks/k", blocked: true,
		},
		{
			// AC6: missing plugin id segment — not a real /:id/webhooks/:key route.
			name: "plugin webhooks with no id segment", method: http.MethodPost,
			path: "/api/plugins/webhooks/x", blocked: true,
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
	svc, _ := newTestService(t, true)
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
	svc, _ := newTestService(t, true)
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

// TestEnabledModeScopedSessionCookieAuthenticates pins the middleware's
// request-aware cookie resolution: on a ported Host it reads the
// port-scoped session cookie name.
func TestEnabledModeScopedSessionCookieAuthenticates(t *testing.T) {
	svc, _ := newTestService(t, true)
	token := setupAdmin(t, svc)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.Host = "127.0.0.1:8443"
		r.AddCookie(&http.Cookie{Name: "kandev_session_8443", Value: token})
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("probe with scoped session: %d body=%s", rec.Code, rec.Body.String())
	}
	if contains(rec.Body.String(), `"synthetic":true`) {
		t.Fatal("real session must not be synthetic")
	}
}

// TestEnabledModeRejectsForeignPortSessionCookie pins per-port resolution: a
// token carried under another port's scoped name is not read for this Host.
func TestEnabledModeRejectsForeignPortSessionCookie(t *testing.T) {
	svc, _ := newTestService(t, true)
	token := setupAdmin(t, svc)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/probe", func(r *http.Request) {
		r.Host = "127.0.0.1:8443"
		r.AddCookie(&http.Cookie{Name: "kandev_session_9443", Value: token})
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("probe with foreign-port session: %d, want 401", rec.Code)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// echoXFH handler reports whether X-Forwarded-Host reached the handler.
// echoXFH reports whether X-Forwarded-Host reached the handler.
func echoXFH(c *gin.Context) {
	if xfh := c.GetHeader("X-Forwarded-Host"); xfh != "" {
		c.String(http.StatusOK, xfh)
		return
	}
	c.String(http.StatusOK, "stripped")
}

// stripRouter builds a gin router with the X-Forwarded-Host strip middleware
// (trusted list as given) and an echoXFH handler that reports what reached it.
func stripRouter(trusted ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(StripUntrustedForwardedHost(trusted, mustNopLogger()))
	router.GET("/", echoXFH)
	return router
}

// mustNopLogger returns a discard logger for strip-middleware tests.
func mustNopLogger() *logger.Logger {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		panic(err)
	}
	return log
}

// TestStripUntrustedForwardedHostUntrustedPeerStrips pins the secure default:
// without a trusted-proxy list, an X-Forwarded-Host presented by the peer is
// dropped before the handler (and the cookie resolver) sees it.
func TestStripUntrustedForwardedHostUntrustedPeerStrips(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter().ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("untrusted XFH body = %q, want stripped", got)
	}
}

// TestStripUntrustedForwardedHostTrustedPeerKeeps pins the opt-in behavior: a
// peer inside the trusted list may carry X-Forwarded-Host (the proxy-authored
// browser host:port) through to the cookie resolver.
func TestStripUntrustedForwardedHostTrustedPeerKeeps(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:5555"
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter("10.0.0.0/8").ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "public.example:8443" {
		t.Fatalf("trusted XFH body = %q, want public.example:8443", got)
	}
}

// TestStripUntrustedForwardedHostIPv4MappedPeerKeeps pins the IPv4-mapped
// IPv6 equivalence: gin compares the IPv4 form of an IPv4-mapped peer against
// the trusted list, and the matcher must agree (fail-closed here would break
// the port-rewrite cookie flow for dual-stack proxies).
func TestStripUntrustedForwardedHostIPv4MappedPeerKeeps(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::ffff:10.0.0.5]:5555"
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter("10.0.0.0/8", "10.0.0.5").ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "public.example:8443" {
		t.Fatalf("IPv4-mapped trusted XFH body = %q, want public.example:8443", got)
	}
}

// TestStripUntrustedForwardedHostMappedFormCIDRKeeps pins the mapped-form
// CIDR normalization: gin treats ::ffff:10.0.0.0/120 as 10.0.0.0/24 and
// trusts peers under it in either representation; the matcher must agree.
func TestStripUntrustedForwardedHostMappedFormCIDRKeeps(t *testing.T) {
	for _, peer := range []string{"10.0.0.5:5555", "[::ffff:10.0.0.5]:5555"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = peer
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		rec := httptest.NewRecorder()
		stripRouter("::ffff:10.0.0.0/120").ServeHTTP(rec, req)
		if got := rec.Body.String(); got != "public.example:8443" {
			t.Fatalf("mapped-form CIDR peer %q: XFH body = %q, want public.example:8443", peer, got)
		}
	}
}

// TestStripUntrustedForwardedHostDegenerateMappedCIDRFailsClosed pins the
// unsupported corner: a mapped-form CIDR with fewer than 96 bits matches no
// IPv4-form peer in gin (dead entry); the matcher mirrors that by skipping it
// (fail closed) rather than silently trusting something gin would not.
func TestStripUntrustedForwardedHostDegenerateMappedCIDRFailsClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter("::ffff:10.0.0.0/90").ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("degenerate mapped CIDR body = %q, want stripped", got)
	}
}

// TestStripUntrustedForwardedHostMappedForm96MirrorsGin pins the /96
// boundary: a mapped-form /96 degenerates to 0.0.0.0/0 in gin (trusts every
// IPv4 peer), and the matcher mirrors that trust instead of failing closed —
// failing closed there would strip a header gin trusts and break the
// port-rewrite cookie flow for the config.
func TestStripUntrustedForwardedHostMappedForm96MirrorsGin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5555" // any IPv4 peer
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter("::ffff:10.0.0.0/96").ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "public.example:8443" {
		t.Fatalf("mapped /96 body = %q, want public.example:8443 (gin trusts all IPv4)", got)
	}
}

// TestStripUntrustedForwardedHostCIDRExcludesOutOfRangePeer pins the CIDR
// boundary: the peer just outside the trusted prefix is untrusted.
func TestStripUntrustedForwardedHostCIDRExcludesOutOfRangePeer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.7:5555"
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	stripRouter("192.168.0.0/24").ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("out-of-CIDR XFH body = %q, want stripped", got)
	}
}

// TestStripUntrustedForwardedHostNoHeaderUntouched pins the no-op path: a
// request without the header passes through unchanged.
func TestStripUntrustedForwardedHostNoHeaderUntouched(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	stripRouter().ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("no-header body = %q, want stripped (handler default)", got)
	}
}

// TestStripUntrustedForwardedHostWarns pins the operator signal: stripping an
// untrusted X-Forwarded-Host emits a warning naming the peer, so a
// misconfigured proxy (or a client trying to pick its own cookie scope) is
// visible in the logs instead of silently no-opping.
func TestStripUntrustedForwardedHostWarns(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(StripUntrustedForwardedHost(nil, log))
	router.GET("/", echoXFH)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-Host", "public.example:8443")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("warn-test body = %q, want stripped", got)
	}
	warned := false
	for _, entry := range logs.All() {
		if !strings.Contains(entry.Message, "X-Forwarded-Host") {
			continue
		}
		warned = true
		for _, field := range entry.Context {
			if field.Key == "peer" && field.String != "203.0.113.9" {
				t.Fatalf("warning peer = %q, want 203.0.113.9", field.String)
			}
		}
	}
	if !warned {
		t.Fatal("no X-Forwarded-Host warning was logged")
	}
	assertWarningHints(t, logs)
}

func assertWarningHints(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	for i, entry := range logs.All() {
		hint := ""
		for _, field := range entry.Context {
			if field.Key == "hint" {
				hint = field.String
				break
			}
		}
		if hint != forwardedHostWarnHint {
			t.Fatalf("warning %d hint = %q, want %q", i, hint, forwardedHostWarnHint)
		}
	}
}

// stripWarnRouter builds a strip-middleware router over an observed logger and
// returns both, so a test can count the warnings a request sequence produces.
func stripWarnRouter(t *testing.T) (*gin.Engine, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(StripUntrustedForwardedHost(nil, log))
	router.GET("/", echoXFH)
	return router, logs
}

// serveStrip sends one request with the given peer and X-Forwarded-Host,
// asserting the header was stripped so warn counting never masks a behavior
// regression.
func serveStrip(t *testing.T, router *gin.Engine, peer, forwardedHost string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	req.Header.Set("X-Forwarded-Host", forwardedHost)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "stripped" {
		t.Fatalf("peer %s host %s: body = %q, want stripped", peer, forwardedHost, got)
	}
}

// TestStripUntrustedForwardedHostWarnsOncePerPeer pins the deduplication: an
// untrusted proxy forwards on every request, so warning per request buries the
// log in thousands of identical lines. The first request warns; repeats from
// the same peer and host stay silent, and the header is still stripped.
func TestStripUntrustedForwardedHostWarnsOncePerPeer(t *testing.T) {
	router, logs := stripWarnRouter(t)
	for range 50 {
		serveStrip(t, router, "203.0.113.9:5555", "public.example:8443")
	}
	if got := logs.Len(); got != 1 {
		t.Fatalf("warnings for 50 identical requests = %d, want 1", got)
	}
	assertWarningHints(t, logs)
}

// TestStripUntrustedForwardedHostWarnsPerDistinctPair pins the other half: a
// new peer, or a known peer presenting a new forwarded host, is genuinely new
// information and warns again.
func TestStripUntrustedForwardedHostWarnsPerDistinctPair(t *testing.T) {
	router, logs := stripWarnRouter(t)
	serveStrip(t, router, "203.0.113.9:5555", "public.example:8443")
	serveStrip(t, router, "203.0.113.9:5555", "public.example:8443") // repeat: silent
	serveStrip(t, router, "198.51.100.4:5555", "public.example:8443")
	serveStrip(t, router, "203.0.113.9:5555", "other.example:8443")
	if got := logs.Len(); got != 3 {
		t.Fatalf("warnings for 3 distinct pairs = %d, want 3", got)
	}
	assertWarningHints(t, logs)
}

func TestForwardedHostWarnSetFirstIsAtMostOnce(t *testing.T) {
	set := newForwardedHostWarnSet()
	const calls = 100
	results := make(chan bool, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- set.first("203.0.113.9", "public.example:8443")
		}()
	}
	wg.Wait()
	close(results)

	first := 0
	for result := range results {
		if result {
			first++
		}
	}
	if first != 1 {
		t.Fatalf("first returned true %d times, want 1", first)
	}
}

// TestStripUntrustedForwardedHostWarnSetIsBounded pins the memory bound: a
// client that varies the forwarded host cannot grow the dedup set without
// limit. Warnings stop at the cap; stripping does not.
func TestStripUntrustedForwardedHostWarnSetIsBounded(t *testing.T) {
	router, logs := stripWarnRouter(t)
	for i := range forwardedHostWarnLimit * 2 {
		serveStrip(t, router, "203.0.113.9:5555", fmt.Sprintf("host-%d.example:8443", i))
	}
	if got := logs.Len(); got != forwardedHostWarnLimit {
		t.Fatalf("warnings for %d distinct hosts = %d, want %d (cap)",
			forwardedHostWarnLimit*2, got, forwardedHostWarnLimit)
	}
}

// oversizedHost builds a forwarded host near net/http's default 1 MiB header
// budget, which backendapp leaves unset: the largest value an unauthenticated
// client can actually get past the server.
func oversizedHost(seed int) string {
	return fmt.Sprintf("%d.", seed) + strings.Repeat("a", 1<<20-16) + ".example:8443"
}

// TestStripUntrustedForwardedHostRetainsFixedSizeKeys pins the memory bound in
// bytes, not just in entries: the forwarded host is unauthenticated and can
// approach the whole header budget, so retaining raw pair values would let 64
// oversized requests pin tens of MiB for the process lifetime despite the entry
// cap. Keys are fixed-size digests, so retention does not scale with header
// size, and distinct oversized hosts still dedup independently.
func TestStripUntrustedForwardedHostRetainsFixedSizeKeys(t *testing.T) {
	set := newForwardedHostWarnSet()
	for i := range forwardedHostWarnLimit {
		host := oversizedHost(i)
		if !set.first("203.0.113.9", host) {
			t.Fatalf("first oversized host %d: got false, want true", i)
		}
		if set.first("203.0.113.9", host) {
			t.Fatalf("repeat oversized host %d: got true, want false", i)
		}
	}
	retained := 0
	for key := range set.seen {
		retained += len(key)
	}
	if want := forwardedHostWarnLimit * sha256.Size; retained != want {
		t.Fatalf("retained key bytes = %d, want %d (fixed-size digests)", retained, want)
	}
}

// TestStripUntrustedForwardedHostDistinguishesLongSharedPrefixes pins what the
// digest buys over simply retaining a fixed-size prefix of the pair: hosts that
// differ only past the key width must still be told apart. A prefix key would
// collide here and silently swallow the second host's warning, which is the one
// an operator most needs when a proxy starts forwarding something new.
func TestStripUntrustedForwardedHostDistinguishesLongSharedPrefixes(t *testing.T) {
	shared := strings.Repeat("a", 1<<20-64)
	router, logs := stripWarnRouter(t)
	serveStrip(t, router, "203.0.113.9:5555", shared+"-one.example:8443")
	serveStrip(t, router, "203.0.113.9:5555", shared+"-two.example:8443")
	if got := logs.Len(); got != 2 {
		t.Fatalf("warnings for 2 hosts sharing a %d-byte prefix = %d, want 2", len(shared), got)
	}
}

// TestStripUntrustedForwardedHostTruncatesLoggedHost pins the other half of the
// same bound: an oversized header must not be echoed into the log verbatim,
// which would recreate in one line the flooding this deduplication exists to
// stop. The value is still stripped and still warned about once.
func TestStripUntrustedForwardedHostTruncatesLoggedHost(t *testing.T) {
	router, logs := stripWarnRouter(t)
	host := oversizedHost(0)
	serveStrip(t, router, "203.0.113.9:5555", host)
	serveStrip(t, router, "203.0.113.9:5555", host)
	if got := logs.Len(); got != 1 {
		t.Fatalf("warnings for repeated oversized host = %d, want 1", got)
	}
	logged := ""
	for _, field := range logs.All()[0].Context {
		if field.Key == "forwarded_host" {
			logged = field.String
		}
	}
	if want := forwardedHostLogMaxLen + len("...(truncated)"); len(logged) > want {
		t.Fatalf("logged forwarded_host = %d bytes, want <= %d", len(logged), want)
	}
	if !strings.HasSuffix(logged, "...(truncated)") {
		t.Fatalf("logged forwarded_host = %q, want a truncation marker", logged)
	}
	if !strings.HasPrefix(host, strings.TrimSuffix(logged, "...(truncated)")) {
		t.Fatalf("logged forwarded_host %q is not a prefix of the header", logged)
	}
}

// TestTruncateForLogPreservesShortValuesAndRuneBoundaries pins the two edges of
// the truncation helper: a normal hostname is logged verbatim, and an oversized
// multi-byte value is cut on a rune boundary so the log field stays valid UTF-8.
func TestTruncateForLogPreservesShortValuesAndRuneBoundaries(t *testing.T) {
	if got := truncateForLog("public.example:8443"); got != "public.example:8443" {
		t.Fatalf("short value = %q, want it unchanged", got)
	}
	// "é" is 2 bytes, so the cap lands mid-rune for at least one repeat count.
	for _, extra := range []int{0, 1} {
		value := strings.Repeat("é", forwardedHostLogMaxLen/2+extra+8)
		got := truncateForLog(value)
		if !utf8.ValidString(got) {
			t.Fatalf("truncated multi-byte value is not valid UTF-8: %q", got)
		}
	}
}

// TestSessionIPRefreshedThroughMiddleware proves the full chain gin →
// middleware → service → store for the session-IP refresh, in four transport
// sub-cases: (a) RemoteAddr-only on a cleared-proxy router, with a real-route
// read-back of GET /api/v1/auth/sessions; (b) a trusted peer whose
// X-Forwarded-For wins over X-Real-IP; (c) an untrusted peer whose forwarded
// header is ignored; (d) a cookie-authenticated deferred-path request (/ws)
// still refreshes. Each sub-case runs on a FRESH harness with its own aged
// session so a resolve cannot be throttled by a sibling's touch.
func TestSessionIPRefreshedThroughMiddleware(t *testing.T) {
	// createAgedSession seeds a session whose last_seen_at is ≥2 minutes in
	// the past (the 1-minute touch interval is unexported in package auth,
	// so this recipe cannot name it) with a 24h future expiry so a slow
	// test cannot trip the delete-before-touch path, under the active
	// DefaultUserID row seeded by userstore.Provide. setupAdmin is NOT
	// called: it would mint a second DefaultUserID session.
	createAgedSession := func(t *testing.T, store *authstore.Store) (token, hash string) {
		t.Helper()
		token, hash, err := auth.GenerateSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		session := &authstore.Session{
			UserID: userstore.DefaultUserID, TokenHash: hash,
			CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), LastSeenAt: now.Add(-2 * time.Hour),
			IP: "1.1.1.1",
		}
		if err := store.CreateSession(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		return token, hash
	}
	// sessionIP selects by TokenHash, never list index, so a future fixture
	// extension cannot silently break the assertions.
	sessionIP := func(t *testing.T, svc *auth.Service, hash string) string {
		t.Helper()
		sessions, err := svc.ListSessions(context.Background(), userstore.DefaultUserID)
		if err != nil {
			t.Fatalf("list sessions: %v", err)
		}
		for _, s := range sessions {
			if s.TokenHash == hash {
				return s.IP
			}
		}
		t.Fatalf("no session row for hash %s", hash)
		return ""
	}
	withCookie := func(svc *auth.Service, token string) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: svc.CookieName(), Value: token})
		}
	}

	t.Run("cleared proxies uses RemoteAddr", func(t *testing.T) {
		svc, store := newTestService(t, true)
		token, hash := createAgedSession(t, store)
		router := newTestRouter(svc)

		// Probe first (assert its 200), then the stored IP, then the
		// real-route read-back: the read-back runs after the probe's touch
		// so a buggy expires=now touch 401s the read-back's resolve, where
		// the JSON assertion lives.
		rec := doRequest(router, http.MethodGet, "/api/v1/probe", withCookie(svc, token), func(r *http.Request) {
			r.RemoteAddr = "2.2.2.2:1234"
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("probe: %d body=%s", rec.Code, rec.Body.String())
		}
		if got := sessionIP(t, svc, hash); got != "2.2.2.2" {
			t.Fatalf("stored IP = %q, want 2.2.2.2", got)
		}

		log, err := logger.NewFromZap(zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		authhttpapi.RegisterRoutes(router, svc, log)
		rec = doRequest(router, http.MethodGet, "/api/v1/auth/sessions", withCookie(svc, token), func(r *http.Request) {
			r.RemoteAddr = "2.2.2.2:1234"
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("sessions read-back: %d body=%s (expires=now bug or resolution failure)", rec.Code, rec.Body.String())
		}
		var out struct {
			Sessions []struct {
				IP      string `json:"ip"`
				Current bool   `json:"current"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode sessions: %v", err)
		}
		gotIP := ""
		for _, s := range out.Sessions {
			if s.Current {
				gotIP = s.IP
			}
		}
		if gotIP != "2.2.2.2" {
			t.Fatalf("read-back IP = %q, want 2.2.2.2", gotIP)
		}
	})

	t.Run("trusted peer honors X-Forwarded-For over X-Real-IP", func(t *testing.T) {
		svc, store := newTestService(t, true)
		token, hash := createAgedSession(t, store)
		router := newTestRouter(svc, "10.0.0.0/8")

		rec := doRequest(router, http.MethodGet, "/api/v1/probe", withCookie(svc, token), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.5:1234"
			r.Header.Set("X-Forwarded-For", "2.2.2.2")
			r.Header.Set("X-Real-IP", "9.9.9.9")
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("probe: %d body=%s", rec.Code, rec.Body.String())
		}
		// A RemoteAddr-wired middleware stores 10.0.0.5; an X-Real-IP-wired
		// one stores 9.9.9.9; only c.ClientIP() with gin's header
		// precedence stores 2.2.2.2.
		if got := sessionIP(t, svc, hash); got != "2.2.2.2" {
			t.Fatalf("stored IP = %q, want 2.2.2.2", got)
		}
	})

	t.Run("untrusted peer ignores X-Forwarded-For", func(t *testing.T) {
		svc, store := newTestService(t, true)
		token, hash := createAgedSession(t, store)
		router := newTestRouter(svc) // cleared proxies, not gin's trust-all default

		rec := doRequest(router, http.MethodGet, "/api/v1/probe", withCookie(svc, token), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.5:1234"
			r.Header.Set("X-Forwarded-For", "2.2.2.2")
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("probe: %d body=%s", rec.Code, rec.Body.String())
		}
		// A middleware reading the raw header would store 2.2.2.2; the
		// untrusted header must be ignored and the TCP peer recorded.
		if got := sessionIP(t, svc, hash); got != "10.0.0.5" {
			t.Fatalf("stored IP = %q, want 10.0.0.5", got)
		}
	})

	t.Run("deferred path request still refreshes", func(t *testing.T) {
		svc, store := newTestService(t, true)
		token, hash := createAgedSession(t, store)
		router := newTestRouter(svc)

		// /ws is deferred: the middleware never 401s it and this router
		// registers no gateway route, so 404 is NOT a resolution proof —
		// the stored-IP assertion below is the real discriminator (only a
		// successful resolve touches the session row).
		rec := doRequest(router, http.MethodGet, "/ws", withCookie(svc, token), func(r *http.Request) {
			r.RemoteAddr = "2.2.2.2:1234"
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("deferred /ws = %d, want 404 pass-through", rec.Code)
		}
		if got := sessionIP(t, svc, hash); got != "2.2.2.2" {
			t.Fatalf("stored IP = %q, want 2.2.2.2 (deferred-path resolve must refresh)", got)
		}
	})
}
