package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newEnabledAuthService builds an auth service in ModeEnabled (features.auth
// on + an admin identity exists) over an in-memory store pair — the state the
// plugin SSO bridge requires.
func newEnabledAuthService(t *testing.T, cfg *config.Config) *auth.Service {
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
	st, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: st, Users: users})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	if _, _, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatalf("setup admin: %v", err)
	}
	if svc.Mode() != auth.ModeEnabled {
		t.Fatalf("mode = %s, want enabled", svc.Mode())
	}
	return svc
}

// ssoBridgeCookieNames runs the real pluginSSOBridge LoginExternal on a Gin
// context whose request Host carries the given host, and returns the session
// cookie names set on the response.
func ssoBridgeCookieNames(t *testing.T, svc *auth.Service, host string) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/plugins/p/webhooks/cb", nil)
	c.Request.Host = host

	bridge := pluginSSOBridge{auth: svc}
	if err := bridge.LoginExternal(c, "oidc:iss", "sub-1", "alice@x.dev", "Alice"); err != nil {
		t.Fatalf("LoginExternal: %v", err)
	}
	var names []string
	for _, cookie := range rec.Result().Cookies() {
		names = append(names, cookie.Name)
	}
	return names
}

// TestPluginSSOBridgeSetsScopedSessionCookie drives the REAL bridge
// (backendapp/auth.go), not the plugins-package fake: LoginExternal on a Gin
// request Host 127.0.0.1:8443 must set kandev_session_8443. A stale bridge
// call to b.auth.CookieName() would pass resolver tests while emitting the
// plain name for ported SSO.
func TestPluginSSOBridgeSetsScopedSessionCookie(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "127.0.0.1:8443")
	if len(names) != 1 || names[0] != "kandev_session_8443" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [kandev_session_8443]", names)
	}
}

// TestPluginSSOBridgeCustomCookieNameVerbatim pins the custom-name contract
// through the real bridge: a configured auth.cookieName is set verbatim on a
// ported Host (never suffixed).
func TestPluginSSOBridgeCustomCookieNameVerbatim(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	cfg.Auth.CookieName = "sso_session"
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "127.0.0.1:8443")
	if len(names) != 1 || names[0] != "sso_session" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [sso_session]", names)
	}
}

// TestPluginSSOBridgeDefaultPortPlainName pins the no-port branch through the
// real bridge: a default-port Host keeps the plain base name.
func TestPluginSSOBridgeDefaultPortPlainName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "example.com")
	if len(names) != 1 || names[0] != "kandev_session" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [kandev_session]", names)
	}
}
