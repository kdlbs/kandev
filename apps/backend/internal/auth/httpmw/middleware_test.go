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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
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

func TestEnabledModePATAuthenticatesPluginWebhook(t *testing.T) {
	svc := newTestService(t, true)
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
			// AC-14 (docs/specs/health-endpoint-version/spec.md): /health
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

// TestEnabledModeScopedSessionCookieAuthenticates pins the middleware's
// request-aware cookie resolution: on a ported Host it reads the
// port-scoped session cookie name.
func TestEnabledModeScopedSessionCookieAuthenticates(t *testing.T) {
	svc := newTestService(t, true)
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
	svc := newTestService(t, true)
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
}
