package httpmw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

var credentialHTTPSequence atomic.Uint64

type credentialHTTPFixture struct {
	svc        *auth.Service
	writer     *sqlx.DB
	reader     *sqlx.DB
	dsn        string
	adminToken string
}

func newCredentialHTTPFixture(t *testing.T, authEnabled bool) *credentialHTTPFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:auth-http-%d?mode=memory&cache=shared", credentialHTTPSequence.Add(1))
	writer := openCredentialHTTPDB(t, dsn)
	reader := openCredentialHTTPDB(t, dsn)
	f := &credentialHTTPFixture{writer: writer, reader: reader, dsn: dsn}
	f.svc = f.newService(t, authEnabled)
	if authEnabled {
		_, token, err := f.svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", "")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		f.adminToken = token
	}
	return f
}

func (f *credentialHTTPFixture) newService(t *testing.T, authEnabled bool) *auth.Service {
	t.Helper()
	users, cleanup, err := userstore.Provide(f.writer, f.reader)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	authStore, err := authstore.New(f.writer, f.reader)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = authEnabled
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{
		Cfg: cfg, Store: authStore, Users: users,
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	return svc
}

func openCredentialHTTPDB(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (f *credentialHTTPFixture) closeReader() {
	_ = f.reader.Close()
}

func (f *credentialHTTPFixture) mintPAT(t *testing.T) string {
	t.Helper()
	_, pat, err := f.svc.MintToken(context.Background(), userstore.DefaultUserID, "fixture", 0)
	if err != nil {
		t.Fatalf("mint PAT: %v", err)
	}
	return pat
}

// credentialRouter registers the paths used by the route-compatibility matrix
// and counts how often a downstream handler actually ran.
func credentialRouter(svc *auth.Service, calls *atomic.Int64, paths ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(svc))
	handler := func(c *gin.Context) {
		calls.Add(1)
		c.Status(http.StatusOK)
	}
	for _, path := range paths {
		router.GET(path, handler)
		router.POST(path, handler)
	}
	return router
}

func credentialPaths() []string {
	return []string{
		"/api/v1/probe",
		"/api/v1/auth/me",
		"/api/v1/app-state",
		"/api/v1/features",
		"/mcp",
		"/health",
		"/api/v1/office/probe",
	}
}

func credentialRequest(svc *auth.Service, token, path string, ctx context.Context) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: svc.CookieName(), Value: token})
	}
	return req
}

func serveCredentialRequest(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertUnavailableResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("503 body is not JSON (%q): %v", rec.Body.String(), err)
	}
	if body["error"] != "authentication temporarily unavailable" {
		t.Fatalf("503 error = %q", body["error"])
	}
	if body["code"] != "persistence_unavailable" {
		t.Fatalf("503 code = %q", body["code"])
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("503 must not challenge, WWW-Authenticate = %q", got)
	}
	if got := rec.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("503 must not clear or reset the session cookie, Set-Cookie = %q", got)
	}
}

func assertAuthenticationChallenge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
	}
}

func TestCredentialStoreFailureReturns503(t *testing.T) {
	cases := []struct {
		name  string
		apply func(t *testing.T, f *credentialHTTPFixture, req *http.Request)
	}{
		{
			name: "session cookie",
			apply: func(_ *testing.T, f *credentialHTTPFixture, req *http.Request) {
				req.AddCookie(&http.Cookie{Name: f.svc.CookieName(), Value: f.adminToken})
			},
		},
		{
			name: "PAT bearer",
			apply: func(t *testing.T, f *credentialHTTPFixture, req *http.Request) {
				req.Header.Set("Authorization", "Bearer "+f.mintPAT(t))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCredentialHTTPFixture(t, true)
			var calls atomic.Int64
			router := credentialRouter(f.svc, &calls, "/api/v1/probe")

			baseline := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
			tc.apply(t, f, baseline)
			if rec := serveCredentialRequest(router, baseline); rec.Code != http.StatusOK {
				t.Fatalf("baseline status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
			}
			if calls.Load() != 1 {
				t.Fatalf("baseline handler calls = %d, want 1", calls.Load())
			}

			f.closeReader()
			failing := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
			tc.apply(t, f, failing)
			rec := serveCredentialRequest(router, failing)
			assertUnavailableResponse(t, rec)
			if calls.Load() != 1 {
				t.Fatalf("protected handler ran after lookup failure: calls = %d", calls.Load())
			}
		})
	}
}

func TestCredentialFallbackKeepsLookupErrors(t *testing.T) {
	t.Run("valid cookie wins without reading the bearer", func(t *testing.T) {
		f := newCredentialHTTPFixture(t, true)
		req := credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil)
		req.Header.Set("Authorization", "Bearer kandev_pat_invalid")
		var calls atomic.Int64
		rec := serveCredentialRequest(credentialRouter(f.svc, &calls, "/api/v1/probe"), req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid cookie plus valid PAT authenticates", func(t *testing.T) {
		f := newCredentialHTTPFixture(t, true)
		req := credentialRequest(f.svc, "not-a-session", "/api/v1/probe", nil)
		req.Header.Set("Authorization", "Bearer "+f.mintPAT(t))
		var calls atomic.Int64
		rec := serveCredentialRequest(credentialRouter(f.svc, &calls, "/api/v1/probe"), req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("unreadable cookie plus valid PAT authenticates", func(t *testing.T) {
		f := newCredentialHTTPFixture(t, true)
		pat := f.mintPAT(t)
		if _, err := f.writer.ExecContext(context.Background(), `DROP TABLE auth_sessions`); err != nil {
			t.Fatalf("drop sessions: %v", err)
		}
		req := credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil)
		req.Header.Set("Authorization", "Bearer "+pat)
		var calls atomic.Int64
		rec := serveCredentialRequest(credentialRouter(f.svc, &calls, "/api/v1/probe"), req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("unreadable cookie plus invalid PAT keeps the lookup error", func(t *testing.T) {
		f := newCredentialHTTPFixture(t, true)
		if _, err := f.writer.ExecContext(context.Background(), `DROP TABLE auth_sessions`); err != nil {
			t.Fatalf("drop sessions: %v", err)
		}
		req := credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil)
		req.Header.Set("Authorization", "Bearer kandev_pat_missing")
		var calls atomic.Int64
		rec := serveCredentialRequest(credentialRouter(f.svc, &calls, "/api/v1/probe"), req)
		assertUnavailableResponse(t, rec)
	})

	t.Run("unreadable cookie plus non-PAT bearer keeps the lookup error", func(t *testing.T) {
		f := newCredentialHTTPFixture(t, true)
		if _, err := f.writer.ExecContext(context.Background(), `DROP TABLE auth_sessions`); err != nil {
			t.Fatalf("drop sessions: %v", err)
		}
		req := credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil)
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.agent.jwt")
		var calls atomic.Int64
		rec := serveCredentialRequest(credentialRouter(f.svc, &calls, "/api/v1/probe"), req)
		assertUnavailableResponse(t, rec)
	})
}

func TestCredentialFailureRouteCompatibility(t *testing.T) {
	f := newCredentialHTTPFixture(t, true)
	var calls atomic.Int64
	router := credentialRouter(f.svc, &calls, credentialPaths()...)
	f.closeReader()

	withCookie := func(path string) *httptest.ResponseRecorder {
		return serveCredentialRequest(router, credentialRequest(f.svc, f.adminToken, path, nil))
	}
	for _, path := range credentialPaths() {
		if rec := withCookie(path); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s with unreadable cookie = %d, want 503 (body=%s)", path, rec.Code, rec.Body.String())
		}
	}

	for _, path := range []string{"/api/v1/auth/me", "/api/v1/app-state", "/api/v1/features", "/mcp", "/health"} {
		if rec := serveCredentialRequest(router, credentialRequest(f.svc, "", path, nil)); rec.Code != http.StatusOK {
			t.Fatalf("credentialless %s = %d, want 200 (body=%s)", path, rec.Code, rec.Body.String())
		}
	}
	assertAuthenticationChallenge(t, serveCredentialRequest(
		router, credentialRequest(f.svc, "", "/api/v1/probe", nil),
	))

	nonPAT := func(path string) *httptest.ResponseRecorder {
		req := credentialRequest(f.svc, "", path, nil)
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.agent.jwt")
		return serveCredentialRequest(router, req)
	}
	for _, path := range []string{"/mcp", "/api/v1/office/probe"} {
		if rec := nonPAT(path); rec.Code != http.StatusOK {
			t.Fatalf("%s with independent bearer = %d, want 200 (body=%s)", path, rec.Code, rec.Body.String())
		}
	}
	assertAuthenticationChallenge(t, nonPAT("/api/v1/probe"))

	disabled := newCredentialHTTPFixture(t, false)
	var disabledCalls atomic.Int64
	disabledRouter := credentialRouter(disabled.svc, &disabledCalls, "/api/v1/probe")
	rec := serveCredentialRequest(disabledRouter, credentialRequest(disabled.svc, "unreadable", "/api/v1/probe", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("disabled mode status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestCredentialStoreRecoveryWithSameCredential(t *testing.T) {
	f := newCredentialHTTPFixture(t, true)
	var calls atomic.Int64
	router := credentialRouter(f.svc, &calls, "/api/v1/probe")

	baseline := serveCredentialRequest(router, credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil))
	if baseline.Code != http.StatusOK {
		t.Fatalf("baseline status = %d", baseline.Code)
	}
	originalCookie := baseline.Header().Get("Set-Cookie")

	f.closeReader()
	failed := serveCredentialRequest(router, credentialRequest(f.svc, f.adminToken, "/api/v1/probe", nil))
	assertUnavailableResponse(t, failed)

	f.reader = openCredentialHTTPDB(t, f.dsn)
	recoveredService := f.newService(t, true)
	var recoveredCalls atomic.Int64
	recoveredRouter := credentialRouter(recoveredService, &recoveredCalls, "/api/v1/probe")
	recovered := serveCredentialRequest(recoveredRouter, credentialRequest(recoveredService, f.adminToken, "/api/v1/probe", nil))
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered status = %d, want 200 (body=%s)", recovered.Code, recovered.Body.String())
	}
	if got := recovered.Header().Get("Set-Cookie"); got != originalCookie {
		t.Fatalf("recovery rewrote the session cookie: baseline %q, recovered %q", originalCookie, got)
	}
}

func TestConcurrentCredentialLookupOutcomes(t *testing.T) {
	f := newCredentialHTTPFixture(t, true)
	var calls atomic.Int64
	router := credentialRouter(f.svc, &calls, "/api/v1/probe")

	const requests = 24
	var success, unavailable atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(fail bool) {
			defer wg.Done()
			ctx := context.Background()
			if fail {
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx = canceled
			}
			rec := serveCredentialRequest(router, credentialRequest(f.svc, f.adminToken, "/api/v1/probe", ctx))
			switch rec.Code {
			case http.StatusOK:
				success.Add(1)
			case http.StatusServiceUnavailable:
				unavailable.Add(1)
			default:
				t.Errorf("status = %d, want 200 or 503 (body=%s)", rec.Code, rec.Body.String())
			}
		}(i%2 == 0)
	}
	wg.Wait()
	if success.Load() != requests/2 || unavailable.Load() != requests/2 {
		t.Fatalf("success=%d unavailable=%d, want %d each", success.Load(), unavailable.Load(), requests/2)
	}
}
