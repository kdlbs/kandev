package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	commonhttpmw "github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// testLogger returns a discard logger for fixtures that must pass one.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("nop logger: %v", err)
	}
	return log
}

// newAuthService builds an auth service from cfg over an in-memory store
// pair. Shared by the plain and CORS-gated fixtures.
func newAuthService(t *testing.T, cfg *config.Config) *auth.Service {
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
	svc, err := auth.NewService(context.Background(), auth.Deps{
		Cfg: cfg, Store: store, Users: users,
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	return svc
}

// newAPIFixture builds the full production HTTP stack for auth: global
// middleware + auth API routes on one router. authEnabled maps to the
// features.auth flag (on ⇒ setup mode until the wizard runs). The router
// mirrors production by default: gin.New() trusts all proxies, so
// SetTrustedProxies is called with the variadic trusted list (nil clears the
// insecure default) exactly like buildHTTPServer does.
func newAPIFixture(t *testing.T, authEnabled bool, trustedProxies ...string) (*gin.Engine, *auth.Service) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Features.Auth = authEnabled
	cfg.Auth.SessionTTLHours = 720
	return newAPIFixtureWithCfg(t, cfg, trustedProxies...)
}

// newAPIFixtureWithCfg is newAPIFixture with an explicit config (custom
// auth.cookieName, loaded defaults, …).
func newAPIFixtureWithCfg(t *testing.T, cfg *config.Config, trustedProxies ...string) (*gin.Engine, *auth.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newAuthService(t, cfg)
	router := gin.New()
	if len(trustedProxies) == 0 {
		trustedProxies = nil
	}
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		t.Fatalf("set trusted proxies: %v", err)
	}
	router.Use(authhttpmw.Middleware(svc))
	RegisterRoutes(router, svc, nil)
	return router, svc
}

// newCORSGatedAPIFixture mirrors the production stack: the CORS/WS origin
// gate (httpmw.AllowedOrigin, the same decision branch as
// newCORSGatedAPIFixture mirrors the production chain for the reverse-proxy
// contract tests: the CORS origin gate (as in backendapp.corsMiddleware) runs
// before the auth middleware, as in buildHTTPServer, and — like
// buildHTTPServer — X-Forwarded-Host is honored only when the request's peer
// is in the given trusted-proxy list (mirroring KANDEV_TRUSTED_PROXIES).
func newCORSGatedAPIFixture(t *testing.T, cfg *config.Config, trustedProxies ...string) (*gin.Engine, *auth.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newAuthService(t, cfg)
	router := gin.New()
	router.Use(authhttpmw.StripUntrustedForwardedHost(trustedProxies, testLogger(t)))
	router.Use(func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			if !commonhttpmw.AllowedOrigin(origin, c.Request.Host) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		c.Next()
	})
	router.Use(authhttpmw.Middleware(svc))
	RegisterRoutes(router, svc, nil)
	return router, svc
}

type apiClient struct {
	t      *testing.T
	router *gin.Engine
	cookie *http.Cookie
	// cookiePrefix filters the session Set-Cookie names this client captures
	// (default "kandev_session"); custom-name tests override it.
	cookiePrefix string
}

func (c *apiClient) do(method, path string, body any, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	for _, m := range mutate {
		m(req)
	}
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	// Capture session cookie updates (login/setup/logout).
	prefix := c.cookiePrefix
	if prefix == "" {
		prefix = "kandev_session"
	}
	for _, cookie := range rec.Result().Cookies() {
		if strings.Contains(cookie.Name, prefix) {
			if cookie.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = &http.Cookie{Name: cookie.Name, Value: cookie.Value}
			}
		}
	}
	return rec
}

// assertSessionCookieNames asserts the response's session cookies (names
// containing filter) are exactly want, in order — a positive-only assertion
// would miss an implementation emitting both the scoped and unprefixed names.
func assertSessionCookieNames(t *testing.T, rec *httptest.ResponseRecorder, filter string, want ...string) {
	t.Helper()
	var got []string
	for _, cookie := range rec.Result().Cookies() {
		if strings.Contains(cookie.Name, filter) {
			got = append(got, cookie.Name)
		}
	}
	if !equalStrings(got, want) {
		t.Fatalf("session Set-Cookie names = %v, want %v", got, want)
	}
}

// equalStrings reports whether two string slices are identical, in order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// setupOn logs the admin in via the setup wizard on the given request Host.
func setupOn(t *testing.T, client *apiClient, host string) *httptest.ResponseRecorder {
	t.Helper()
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	}, func(req *http.Request) { req.Host = host })
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d body=%s", rec.Code, rec.Body.String())
	}
	return rec
}

// loginSessionIP bootstraps the admin via setup, performs a login request
// with the given transport attributes, and returns the stored IP of the
// current session (the login cookie is current; the setup session stays in
// the list and is not selected).
func loginSessionIP(t *testing.T, router *gin.Engine, remoteAddr, forwardedFor, xRealIP string) string {
	t.Helper()
	client := &apiClient{t: t, router: router}
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d body=%s", rec.Code, rec.Body.String())
	}
	// The read-back GET must re-apply the login request's FULL transport
	// closure (RemoteAddr + forwarded headers): the session-IP refresh rides
	// the touch path, so a >60s gap between login and read-back would let
	// the read-back's own resolve overwrite the asserted IP (with the
	// httptest default RemoteAddr, or with the fixture's RemoteAddr where
	// the stored IP came from a forwarded header). The identical transport
	// resolves the same ClientIP, making the touch idempotent.
	transport := func(req *http.Request) {
		req.RemoteAddr = remoteAddr
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		if xRealIP != "" {
			req.Header.Set("X-Real-IP", xRealIP)
		}
	}
	rec = client.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123",
	}, transport)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = client.do(http.MethodGet, "/api/v1/auth/sessions", nil, transport)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions: %d body=%s", rec.Code, rec.Body.String())
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
	for _, s := range out.Sessions {
		if s.Current {
			return s.IP
		}
	}
	t.Fatal("no current session in list")
	return ""
}

// TestLoginSessionIPNoTrustedProxiesIgnoresForwardedFor locks the secure
// default: with no trusted proxies, X-Forwarded-For is ignored and the session
// IP is the TCP peer.
func TestLoginSessionIPNoTrustedProxiesIgnoresForwardedFor(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "203.0.113.7", ""); got != "10.0.0.5" {
		t.Fatalf("session IP = %q, want peer 10.0.0.5", got)
	}
}

// TestLoginSessionIPTrustedPeerUsesForwardedFor locks the opt-in behavior:
// when the TCP peer is in the trusted list, X-Forwarded-For is honored.
func TestLoginSessionIPTrustedPeerUsesForwardedFor(t *testing.T) {
	router, _ := newAPIFixture(t, true, "10.0.0.0/8")
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "203.0.113.7", ""); got != "203.0.113.7" {
		t.Fatalf("session IP = %q, want 203.0.113.7", got)
	}
}

// TestLoginSessionIPUntrustedPeerIgnoresForwardedFor locks the boundary: a
// trusted list that does not contain the peer leaves the header ignored.
func TestLoginSessionIPUntrustedPeerIgnoresForwardedFor(t *testing.T) {
	router, _ := newAPIFixture(t, true, "192.168.0.0/16")
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "203.0.113.7", ""); got != "10.0.0.5" {
		t.Fatalf("session IP = %q, want peer 10.0.0.5", got)
	}
}

// TestLoginSessionIPUsesXRealIP locks the secondary forwarded header: gin
// reads X-Real-IP when X-Forwarded-For is absent.
func TestLoginSessionIPUsesXRealIP(t *testing.T) {
	router, _ := newAPIFixture(t, true, "10.0.0.0/8")
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "", "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("session IP = %q, want 203.0.113.7", got)
	}
}

// TestLoginSessionIPMalformedXFFFallsBackToXRealIP locks gin's fallback: a
// syntactically invalid X-Forwarded-For is skipped and X-Real-IP is read.
func TestLoginSessionIPMalformedXFFFallsBackToXRealIP(t *testing.T) {
	router, _ := newAPIFixture(t, true, "10.0.0.0/8")
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "not-an-ip", "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("session IP = %q, want 203.0.113.7", got)
	}
}

// TestLoginSessionIPNoForwardedHeaderUsesPeer locks the fallback: a trusted
// peer with no forwarded-IP header still records the TCP peer.
func TestLoginSessionIPNoForwardedHeaderUsesPeer(t *testing.T) {
	router, _ := newAPIFixture(t, true, "10.0.0.0/8")
	if got := loginSessionIP(t, router, "10.0.0.5:1234", "", ""); got != "10.0.0.5" {
		t.Fatalf("session IP = %q, want peer 10.0.0.5", got)
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}

// TestFullLifecycle drives the opt-in flow through HTTP with the features.auth
// flag on: setup mode → wizard → cookie session → invite a member → member
// restrictions → logout. Enabling/disabling auth itself is a runtime feature
// flag (Feature Toggles), not an API call, so it is not exercised here.
func TestFullLifecycle(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}

	// Setup mode: anonymous /auth/me reports it, protected APIs are blocked.
	me := decode(t, client.do(http.MethodGet, "/api/v1/auth/me", nil))
	if me["mode"] != "setup" || me["authenticated"] != false {
		t.Fatalf("setup-mode me = %v", me)
	}

	// Setup wizard creates the admin and sets the session cookie.
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d body=%s", rec.Code, rec.Body.String())
	}
	if client.cookie == nil {
		t.Fatal("setup must set the session cookie")
	}

	// Authenticated /auth/me now reports the real admin.
	me = decode(t, client.do(http.MethodGet, "/api/v1/auth/me", nil))
	if me["mode"] != "enabled" || me["authenticated"] != true {
		t.Fatalf("me after setup = %v", me)
	}
	user := me["user"].(map[string]any)
	if user["email"] != "admin@x.dev" || user["role"] != "admin" {
		t.Fatalf("unexpected user %v", user)
	}

	// Admin surfaces work with the cookie.
	rec = client.do(http.MethodGet, "/api/v1/users", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: %d", rec.Code)
	}

	// Mint an invite, accept it as a member with a fresh client.
	rec = client.do(http.MethodPost, "/api/v1/auth/invites", map[string]any{"email": "m@x.dev", "role": "member"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: %d body=%s", rec.Code, rec.Body.String())
	}
	inviteURL := decode(t, rec)["url"].(string)
	token := strings.TrimPrefix(inviteURL, "/invite?token=")

	member := &apiClient{t: t, router: router}
	rec = member.do(http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{
		"token": token, "email": "m@x.dev", "password": "memberpass123", "display_name": "M",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept invite: %d body=%s", rec.Code, rec.Body.String())
	}

	// Member cannot touch admin surfaces.
	if rec := member.do(http.MethodGet, "/api/v1/users", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("member list users: %d, want 403", rec.Code)
	}
	if rec := member.do(http.MethodPost, "/api/v1/auth/invites", map[string]any{"role": "admin"}); rec.Code != http.StatusForbidden {
		t.Fatalf("member create invite: %d, want 403", rec.Code)
	}

	// Member self-service: PAT mint + list.
	rec = member.do(http.MethodPost, "/api/v1/auth/tokens", map[string]any{"name": "cli"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint token: %d body=%s", rec.Code, rec.Body.String())
	}
	pat := decode(t, rec)["token"].(string)
	if !strings.HasPrefix(pat, "kandev_pat_") {
		t.Fatalf("unexpected PAT %q", pat)
	}

	// Logout kills the member session.
	if rec := member.do(http.MethodPost, "/api/v1/auth/logout", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := member.do(http.MethodGet, "/api/v1/auth/tokens", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout tokens: %d, want 401", rec.Code)
	}

	// Wrong-password login is a generic 401.
	rec = client.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "m@x.dev", "password": "wrong-pass!"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: %d", rec.Code)
	}
}

func TestSetupRejectedWhenDisabled(t *testing.T) {
	// Flag off ⇒ disabled mode ⇒ setup unavailable.
	router, _ := newAPIFixture(t, false)
	client := &apiClient{t: t, router: router}
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{"email": "a@b.c", "password": "password123"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("setup in disabled mode: %d, want 409", rec.Code)
	}
}

// TestManagementRoutesRejectSyntheticIdentity locks the P1: while auth is
// disabled the middleware injects a synthetic admin (RoleAdmin). Because
// RequireAdmin alone would let it through, an attacker hitting a not-yet-enabled
// instance could mint a PAT or plant an admin that survives enablement and
// hijacks first-run setup. RequireRealIdentity must reject these with 404.
func TestManagementRoutesRejectSyntheticIdentity(t *testing.T) {
	router, _ := newAPIFixture(t, false) // disabled mode ⇒ synthetic admin identity
	client := &apiClient{t: t, router: router}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/auth/tokens", map[string]any{"name": "pwn"}},
		{http.MethodGet, "/api/v1/auth/tokens", nil},
		{http.MethodGet, "/api/v1/auth/sessions", nil},
		{http.MethodPatch, "/api/v1/auth/password", map[string]any{"current_password": "x", "new_password": "password123"}},
		{http.MethodPost, "/api/v1/users", map[string]any{"email": "evil@b.c", "password": "password123", "role": "admin"}},
		{http.MethodGet, "/api/v1/users", nil},
		{http.MethodPost, "/api/v1/auth/invites", map[string]any{"email": "evil@b.c"}},
	}
	for _, tc := range cases {
		rec := client.do(tc.method, tc.path, tc.body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s under disabled auth: %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
	// Public bootstrap routes stay reachable while disabled.
	if rec := client.do(http.MethodGet, "/api/v1/auth/me", nil); rec.Code != http.StatusOK {
		t.Errorf("/me under disabled auth: %d, want 200", rec.Code)
	}
}

func TestValidationErrors(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{"email": "not-an-email", "password": "password123"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad email: %d", rec.Code)
	}
	rec = client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{"email": "a@b.c", "password": "short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: %d", rec.Code)
	}
}

// TestSessionCookieScopedToPortedHost pins the core isolation contract: login
// on a ported Host sets kandev_session_<port>, a request carrying only the
// legacy unprefixed cookie is rejected (mixed-version tier 1), and the scoped
// cookie authenticates subsequent requests.
func TestSessionCookieScopedToPortedHost(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}
	host := "127.0.0.1:8443"

	rec := setupOn(t, client, host)
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")
	if client.cookie == nil || client.cookie.Name != "kandev_session_8443" {
		t.Fatalf("captured cookie = %+v, want kandev_session_8443", client.cookie)
	}

	// A request carrying only the legacy unprefixed cookie is rejected on the
	// ported host (old→new rejection; pre-change it authenticated).
	legacy := &apiClient{t: t, router: router}
	rec = legacy.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = host
		req.Header.Set("Cookie", "kandev_session="+client.cookie.Value)
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy cookie on ported host: %d, want 401", rec.Code)
	}

	// The scoped cookie authenticates on the ported host (and only there).
	rec = client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = host
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("scoped cookie on ported host: %d, want 200", rec.Code)
	}
	if rec := client.do(http.MethodGet, "/api/v1/users", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("scoped cookie on default-port host: %d, want 401", rec.Code)
	}
}

// TestSessionCookieLogoutClearsScopedName pins the logout/clear path: the
// same scoped name login wrote is cleared, and the unprefixed base name is
// left untouched — on a host serving a default-port instance it is that
// instance's live session cookie (see spec: no proactive legacy scrubbing).
func TestSessionCookieLogoutClearsScopedName(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}
	host := "127.0.0.1:8443"
	setupOn(t, client, host)

	rec := client.do(http.MethodPost, "/api/v1/auth/logout", nil, func(req *http.Request) {
		req.Host = host
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "kandev_session_8443" && cookie.MaxAge >= 0 {
			t.Fatalf("logout did not expire the scoped cookie: %+v", cookie)
		}
	}
	if client.cookie != nil {
		t.Fatal("logout must clear the client cookie")
	}
	if rec := client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = host
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout ported request: %d, want 401", rec.Code)
	}
}

// TestInviteAcceptSetsScopedCookie covers the invite-accept set path (there
// is no separate admin-login handler — the first admin's login flows through
// setup).
func TestInviteAcceptSetsScopedCookie(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	admin := &apiClient{t: t, router: router}
	host := "127.0.0.1:8443"
	setupOn(t, admin, host)

	rec := admin.do(http.MethodPost, "/api/v1/auth/invites", map[string]any{"email": "m@x.dev", "role": "member"},
		func(req *http.Request) { req.Host = host })
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: %d body=%s", rec.Code, rec.Body.String())
	}
	token := strings.TrimPrefix(decode(t, rec)["url"].(string), "/invite?token=")

	member := &apiClient{t: t, router: router}
	rec = member.do(http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{
		"token": token, "email": "m@x.dev", "password": "memberpass123", "display_name": "M",
	}, func(req *http.Request) { req.Host = host })
	if rec.Code != http.StatusOK {
		t.Fatalf("accept invite: %d body=%s", rec.Code, rec.Body.String())
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")
	if member.cookie == nil || member.cookie.Name != "kandev_session_8443" {
		t.Fatalf("member cookie = %+v, want kandev_session_8443", member.cookie)
	}
}

// TestSessionCookieSuffixWithLoadedDefaultConfig pins the config-default
// regression: with the loaded default config (empty auth.cookieName) login on
// a ported Host still sets kandev_session_<port> — a seeded non-empty default
// must not suppress suffixing in production.
func TestSessionCookieSuffixWithLoadedDefaultConfig(t *testing.T) {
	t.Setenv("KANDEV_AUTH_COOKIE_NAME", "")
	cfg, err := config.LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Auth.CookieName != "" {
		t.Fatalf("loaded default auth.cookieName = %q, want empty", cfg.Auth.CookieName)
	}
	cfg.Features.Auth = true

	router, _ := newAPIFixtureWithCfg(t, cfg)
	client := &apiClient{t: t, router: router}
	rec := setupOn(t, client, "127.0.0.1:8443")
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")
}

// TestCustomCookieNameUsedVerbatim pins the custom-name contract: an
// explicitly configured auth.cookieName is never suffixed, and authenticates
// through the same middleware path.
func TestCustomCookieNameUsedVerbatim(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	cfg.Auth.CookieName = "my_session"
	router, _ := newAPIFixtureWithCfg(t, cfg)
	client := &apiClient{t: t, router: router, cookiePrefix: "my_session"}

	rec := setupOn(t, client, "127.0.0.1:8443")
	assertSessionCookieNames(t, rec, "my_session", "my_session")
	if client.cookie == nil || client.cookie.Name != "my_session" {
		t.Fatalf("captured cookie = %+v, want my_session (verbatim)", client.cookie)
	}
	rec = client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:8443"
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("custom-name cookie on ported host: %d, want 200", rec.Code)
	}
}

// TestSessionCookiePlainOnDefaultPort covers the no-port branch through the
// real handlers: login on a default-port Host sets the plain kandev_session,
// the middleware authenticates with it, and logout clears it (invariant
// back-compat guard: identical before and after the change).
func TestSessionCookiePlainOnDefaultPort(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}

	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d body=%s", rec.Code, rec.Body.String())
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session")
	if client.cookie == nil || client.cookie.Name != "kandev_session" {
		t.Fatalf("captured cookie = %+v, want plain kandev_session", client.cookie)
	}
	if rec := client.do(http.MethodGet, "/api/v1/users", nil); rec.Code != http.StatusOK {
		t.Fatalf("default-port session: %d, want 200", rec.Code)
	}
	rec = client.do(http.MethodPost, "/api/v1/auth/logout", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session")
}

// TestSessionCookieForeignPortRejectedOnPortedHost is the tier-1b rollback
// guard: a request carrying a cookie under a name the resolver does not
// derive for its Host is rejected both before and after the change, so a
// regression where the middleware matches any scoped name regardless of port
// cannot slip in.
func TestSessionCookieForeignPortRejectedOnPortedHost(t *testing.T) {
	router, _ := newAPIFixture(t, true)
	client := &apiClient{t: t, router: router}
	setupOn(t, client, "127.0.0.1:8443")

	foreign := &apiClient{t: t, router: router}
	rec := foreign.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:8443"
		req.Header.Set("Cookie", "kandev_session_9443="+client.cookie.Value)
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("foreign-port scoped cookie: %d, want 401", rec.Code)
	}
}

// TestTwoInstancesSessionIsolation drives the two-instance behavioral
// regression: independent stores and request Hosts 127.0.0.1:8443 /
// 127.0.0.1:9443. Logging into one must not authenticate (or invalidate) the
// other, and each instance authenticates only with its own port's cookie
// name.
func TestTwoInstancesSessionIsolation(t *testing.T) {
	routerA, _ := newAPIFixture(t, true)
	routerB, _ := newAPIFixture(t, true)
	clientA := &apiClient{t: t, router: routerA}
	clientB := &apiClient{t: t, router: routerB}

	setupOn(t, clientA, "127.0.0.1:8443")
	if clientA.cookie == nil || clientA.cookie.Name != "kandev_session_8443" {
		t.Fatalf("A cookie = %+v, want kandev_session_8443", clientA.cookie)
	}

	// B rejects A's scoped cookie (failing-before: pre-change both wrote the
	// same unprefixed name, so A's token authenticated on B).
	rec := clientB.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:9443"
		req.Header.Set("Cookie", clientA.cookie.Name+"="+clientA.cookie.Value)
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("B with A's cookie: %d, want 401", rec.Code)
	}

	// Logging into B keeps A's session intact.
	setupOn(t, clientB, "127.0.0.1:9443")
	if clientB.cookie == nil || clientB.cookie.Name != "kandev_session_9443" {
		t.Fatalf("B cookie = %+v, want kandev_session_9443", clientB.cookie)
	}
	if rec := clientA.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:8443"
	}); rec.Code != http.StatusOK {
		t.Fatalf("A after B login: %d, want 200 (A's session must survive)", rec.Code)
	}
	if rec := clientB.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:9443"
	}); rec.Code != http.StatusOK {
		t.Fatalf("B after B login: %d, want 200", rec.Code)
	}
}

// authFixtureConfig is the minimal auth-enabled config for the proxy-contract
// fixtures.
func authFixtureConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	return cfg
}

// TestProxyContractPreservedHost drives the compliant reverse-proxy setup (a):
// the proxy preserves the full Host (host:port). The origin gate passes
// (hostname match) — reachability is invariant — and the failing-before
// assertion is the paired Set-Cookie: kandev_session_8443.
func TestProxyContractPreservedHost(t *testing.T) {
	router, _ := newCORSGatedAPIFixture(t, authFixtureConfig())
	client := &apiClient{t: t, router: router}
	host := "public.example:8443"
	origin := "https://public.example:8443"

	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	}, func(req *http.Request) {
		req.Host = host
		req.Header.Set("Origin", origin)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup through preserved Host: %d body=%s", rec.Code, rec.Body.String())
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")

	if rec := client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = host
		req.Header.Set("Origin", origin)
	}); rec.Code != http.StatusOK {
		t.Fatalf("authenticated call through preserved Host: %d, want 200", rec.Code)
	}
}

// TestProxyContractPortRewriteDerivesPublicPortFromXFH drives the compliant
// setup (b): a same-hostname port rewrite with a correct X-Forwarded-Host
// from a TRUSTED reverse proxy (peer inside KANDEV_TRUSTED_PROXIES, as in
// buildHTTPServer). The gate passes (ports are not compared) and the
// failing-before assertion is the cookie resolver deriving _8443 from XFH,
// not _38429 from the internal Host.
func TestProxyContractPortRewriteDerivesPublicPortFromXFH(t *testing.T) {
	router, _ := newCORSGatedAPIFixture(t, authFixtureConfig(), "10.0.0.0/8")
	client := &apiClient{t: t, router: router}

	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	}, func(req *http.Request) {
		req.RemoteAddr = "10.0.0.5:5555" // inside the trusted proxy range
		req.Host = "public.example:38429"
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		req.Header.Set("Origin", "https://public.example:8443")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup through port rewrite: %d body=%s", rec.Code, rec.Body.String())
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_8443")

	if rec := client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.RemoteAddr = "10.0.0.5:5555"
		req.Host = "public.example:38429"
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		req.Header.Set("Origin", "https://public.example:8443")
	}); rec.Code != http.StatusOK {
		t.Fatalf("authenticated call through port rewrite: %d, want 200", rec.Code)
	}
}

// TestProxyContractPortRewriteUntrustedXFHIgnored pins the secure default:
// an UNTRUSTED peer's X-Forwarded-Host is stripped before the cookie resolver
// runs (browsers never send the header, so only proxies or scripts do), and
// the suffix falls back to the request Host. This is the silent no-op the
// pre-fix resolver would have performed; the strip middleware now drops the
// header with a warning instead.
func TestProxyContractPortRewriteUntrustedXFHIgnored(t *testing.T) {
	router, _ := newCORSGatedAPIFixture(t, authFixtureConfig())
	client := &apiClient{t: t, router: router}

	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	}, func(req *http.Request) {
		req.Host = "public.example:38429"
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		req.Header.Set("Origin", "https://public.example:8443")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup through untrusted XFH: %d body=%s", rec.Code, rec.Body.String())
	}
	assertSessionCookieNames(t, rec, "kandev_session", "kandev_session_38429")

	if rec := client.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "public.example:38429"
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		req.Header.Set("Origin", "https://public.example:8443")
	}); rec.Code != http.StatusOK {
		t.Fatalf("authenticated call through untrusted XFH: %d, want 200", rec.Code)
	}
}

// TestProxyContractHostnameRewriteRejected403 drives the non-compliant
// reverse-proxy setup: the proxy rewrites a non-loopback hostname (Host is a
// DIFFERENT non-loopback hostname, not a different port). The origin gate
// compares Origin hostname to Host and ignores X-Forwarded-Host, so the
// request is rejected 403 before auth — an invariant branch that must not be
// weakened by this fix.
func TestProxyContractHostnameRewriteRejected403(t *testing.T) {
	router, _ := newCORSGatedAPIFixture(t, authFixtureConfig())
	client := &apiClient{t: t, router: router}

	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"email": "admin@x.dev", "password": "adminpass123", "display_name": "Admin",
	}, func(req *http.Request) {
		req.Host = "internal:38429" // different non-loopback hostname
		req.Header.Set("X-Forwarded-Host", "public.example:8443")
		req.Header.Set("Origin", "https://public.example:8443")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("hostname rewrite: %d, want 403 (origin gate rejects before auth)", rec.Code)
	}
	assertSessionCookieNames(t, rec, "kandev_session")
}

// TestSameCustomCookieNameStillConflicts pins the documented exception: a
// custom auth.cookieName disables automatic port isolation, so two instances
// configured with the same name on one cookie host write the identical cookie
// name regardless of port (and would overwrite each other in a shared jar).
func TestSameCustomCookieNameStillConflicts(t *testing.T) {
	makeCfg := func() *config.Config {
		cfg := authFixtureConfig()
		cfg.Auth.CookieName = "shared_session"
		return cfg
	}
	routerA, _ := newAPIFixtureWithCfg(t, makeCfg())
	routerB, _ := newAPIFixtureWithCfg(t, makeCfg())
	clientA := &apiClient{t: t, router: routerA, cookiePrefix: "shared_session"}
	clientB := &apiClient{t: t, router: routerB, cookiePrefix: "shared_session"}

	rec := setupOn(t, clientA, "127.0.0.1:8443")
	assertSessionCookieNames(t, rec, "shared_session", "shared_session")
	rec = setupOn(t, clientB, "127.0.0.1:9443")
	assertSessionCookieNames(t, rec, "shared_session", "shared_session")

	// Same name on both ports: no isolation — B's middleware looks up the
	// same cookie name A wrote (a browser jar would hold exactly one).
	if clientA.cookie.Name != clientB.cookie.Name {
		t.Fatalf("custom names differ across ports (%q vs %q), isolation must be disabled by design",
			clientA.cookie.Name, clientB.cookie.Name)
	}
	if rec := clientB.do(http.MethodGet, "/api/v1/users", nil, func(req *http.Request) {
		req.Host = "127.0.0.1:9443"
	}); rec.Code != http.StatusOK {
		t.Fatalf("B's own custom-name session: %d, want 200", rec.Code)
	}
}
