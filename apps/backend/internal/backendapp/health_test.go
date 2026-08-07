package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/system/info"
)

// setReadyForTest flips the package-level readiness flag consulted by
// healthHandler and restores its prior value on cleanup, so tests don't leak
// state into each other or into TestMain-driven suites.
func setReadyForTest(t *testing.T, value bool) {
	t.Helper()
	prev := ready.Load()
	ready.Store(value)
	t.Cleanup(func() { ready.Store(prev) })
}

// TestHealthHandlerReadyBodyIncludesVersion covers AC-1..4: once ready, the
// handler returns 200 with a version key equal to the configured build
// version, alongside the unchanged status/service/mode fields, and no other
// keys.
func TestHealthHandlerReadyBodyIncludesVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, true)

	router := gin.New()
	router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", body["version"])
	}
	if body["status"] != "ok" || body["service"] != "kandev" || body["mode"] != "websocket+http" {
		t.Fatalf("unexpected ready body: %#v", body)
	}
	if len(body) != 4 {
		t.Fatalf("ready body keys = %#v, want exactly status/service/mode/version", body)
	}
}

// TestHealthHandlerStartingBodyIncludesVersion covers AC-5..7: before ready,
// the handler still returns the version alongside status/service, with no
// other keys (mode is ready-path only).
func TestHealthHandlerStartingBodyIncludesVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, false)

	router := gin.New()
	router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", body["version"])
	}
	if body["status"] != "starting" || body["service"] != "kandev" {
		t.Fatalf("unexpected starting body: %#v", body)
	}
	if len(body) != 3 {
		t.Fatalf("starting body keys = %#v, want exactly status/service/version", body)
	}
}

// TestHealthHandlerVersionMatchesSystemInfoVersion covers AC-10: /health and
// /api/v1/system/info are fed the exact same build-version string
// (backendapp.Version, sampled after setBuildInfo — see main.go:801,1830), so
// this pins that both handlers surface it byte-identically rather than each
// having its own notion of "version".
func TestHealthHandlerVersionMatchesSystemInfoVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, true)

	const sameProcessVersion = "9.9.9-test"

	healthRouter := gin.New()
	healthRouter.GET("/health", healthHandler(routeParams{version: sameProcessVersion}))
	healthRec := httptest.NewRecorder()
	healthRouter.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var healthBody map[string]interface{}
	if err := json.Unmarshal(healthRec.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("decode health body: %v", err)
	}

	infoSvc := info.NewService(sameProcessVersion, "commit", "buildtime")
	infoRouter := gin.New()
	infoRouter.GET("/info", info.Handler(infoSvc))
	infoRec := httptest.NewRecorder()
	infoRouter.ServeHTTP(infoRec, httptest.NewRequest(http.MethodGet, "/info", nil))
	var infoBody info.Response
	if err := json.Unmarshal(infoRec.Body.Bytes(), &infoBody); err != nil {
		t.Fatalf("decode info body: %v", err)
	}

	if healthBody["version"] != infoBody.Version {
		t.Fatalf("health version = %v, system/info version = %v, want equal", healthBody["version"], infoBody.Version)
	}
}

// TestSetBuildInfoVersionDefaultAndInjection covers AC-8, AC-9, and AC-11: an
// unstamped build keeps the compiled-in "dev" default, a non-empty ldflag
// value overwrites it, and an empty injected value is ignored rather than
// clearing the version to "".
func TestSetBuildInfoVersionDefaultAndInjection(t *testing.T) {
	prevVersion, prevCommit, prevBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() { Version, Commit, BuildTime = prevVersion, prevCommit, prevBuildTime })

	Version = "dev"
	setBuildInfo(BuildInfo{})
	if Version != "dev" {
		t.Fatalf("Version after empty BuildInfo = %q, want dev default retained", Version)
	}
	if Version == "" {
		t.Fatal("Version must never become empty")
	}

	setBuildInfo(BuildInfo{Version: "2.3.4"})
	if Version != "2.3.4" {
		t.Fatalf("Version after injected BuildInfo = %q, want 2.3.4", Version)
	}

	setBuildInfo(BuildInfo{Version: ""})
	if Version != "2.3.4" {
		t.Fatalf("Version after empty ldflag = %q, want prior value 2.3.4 retained", Version)
	}
}

// TestHealthHandlerAuthEnabledServesVersionWithoutCredential covers AC-12 and
// AC-13: with the auth feature flag on and no credential presented, /health
// must still return its full body (including version) rather than 401/403,
// and the readiness status code/field must be unchanged by auth being on.
func TestHealthHandlerAuthEnabledServesVersionWithoutCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, true)

	svc := newSSOTestAuthService(t)
	if _, _, err := svc.Setup(context.Background(), "admin@example.com", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatalf("Setup admin: %v", err)
	}

	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no auth challenge on /health)", recorder.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", body["version"])
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", body["status"])
	}
}
