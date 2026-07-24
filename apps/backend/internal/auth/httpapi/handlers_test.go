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

	"github.com/kandev/kandev/internal/auth"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newAPIFixture builds the full production HTTP stack for auth: global
// middleware + auth API routes on one router. authEnabled maps to the
// features.auth flag (on ⇒ setup mode until the wizard runs).
func newAPIFixture(t *testing.T, authEnabled bool) (*gin.Engine, *auth.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
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
	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	RegisterRoutes(router, svc, nil)
	return router, svc
}

type apiClient struct {
	t      *testing.T
	router *gin.Engine
	cookie *http.Cookie
}

func (c *apiClient) do(method, path string, body any) *httptest.ResponseRecorder {
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
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	// Capture session cookie updates (login/setup/logout).
	for _, cookie := range rec.Result().Cookies() {
		if strings.Contains(cookie.Name, "kandev_session") {
			if cookie.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = &http.Cookie{Name: cookie.Name, Value: cookie.Value}
			}
		}
	}
	return rec
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
