package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type fakeSettings struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (f *fakeSettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettings) Save(_ context.Context, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[key] = value
	return nil
}

// newAPIFixture builds the full production HTTP stack for auth: global
// middleware + auth API routes on one router.
func newAPIFixture(t *testing.T) (*gin.Engine, *auth.Service) {
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
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{
		Cfg: cfg, Store: store, Users: users, Settings: &fakeSettings{},
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

// TestFullLifecycle drives the entire opt-in flow through HTTP:
// disabled → enable (synthetic admin) → setup wizard → cookie session →
// invite a member → member login and restrictions → logout.
func TestFullLifecycle(t *testing.T) {
	router, svc := newAPIFixture(t)
	client := &apiClient{t: t, router: router}

	// Disabled mode: /auth/me reports the synthetic admin.
	rec := client.do(http.MethodGet, "/api/v1/auth/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d", rec.Code)
	}
	me := decode(t, rec)
	if me["mode"] != "disabled" || me["authenticated"] != true {
		t.Fatalf("disabled-mode me = %v", me)
	}

	// Enable auth (synthetic admin is allowed to).
	rec = client.do(http.MethodPatch, "/api/v1/auth/settings", map[string]any{"enabled": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: %d body=%s", rec.Code, rec.Body.String())
	}
	if body := decode(t, rec); body["setup_required"] != true {
		t.Fatalf("expected setup_required, got %v", body)
	}

	// Setup wizard creates the admin and sets the session cookie.
	rec = client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{
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
	if rec := member.do(http.MethodPatch, "/api/v1/auth/settings", map[string]any{"enabled": false}); rec.Code != http.StatusForbidden {
		t.Fatalf("member disable auth: %d, want 403", rec.Code)
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

	// Admin disables auth again — back to disabled mode.
	rec = client.do(http.MethodPatch, "/api/v1/auth/settings", map[string]any{"enabled": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d", rec.Code)
	}
	if svc.Mode() != auth.ModeDisabled {
		t.Fatalf("mode = %s, want disabled", svc.Mode())
	}
	anonymous := &apiClient{t: t, router: router}
	me = decode(t, anonymous.do(http.MethodGet, "/api/v1/auth/me", nil))
	if me["mode"] != "disabled" || me["authenticated"] != true {
		t.Fatalf("post-disable me = %v", me)
	}
}

func TestSetupRejectedWhenNotInSetupMode(t *testing.T) {
	router, _ := newAPIFixture(t)
	client := &apiClient{t: t, router: router}
	rec := client.do(http.MethodPost, "/api/v1/auth/setup", map[string]any{"email": "a@b.c", "password": "password123"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("setup in disabled mode: %d, want 409", rec.Code)
	}
}

func TestValidationErrors(t *testing.T) {
	router, svc := newAPIFixture(t)
	if _, err := svc.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
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
