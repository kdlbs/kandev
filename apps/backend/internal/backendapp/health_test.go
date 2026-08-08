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

// TestHealthHandlerDesktopTokenHeaderOnlyOnReadyPath guards a pre-existing
// behavior the spec restates as a must-not-regress ("## What": the desktop
// health-token header SHALL continue to be set on the 200 path only,
// unchanged) but that had no direct test before this change extracted the
// handler body into healthHandler. Confirms the refactor didn't move the
// header write outside its original branch.
func TestHealthHandlerDesktopTokenHeaderOnlyOnReadyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(desktopHealthTokenEnv, "  desktop-token  ")

	router := gin.New()
	router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))

	setReadyForTest(t, false)
	startingRec := httptest.NewRecorder()
	router.ServeHTTP(startingRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if got := startingRec.Header().Get(desktopHealthTokenHeader); got != "" {
		t.Fatalf("starting-path %s header = %q, want absent", desktopHealthTokenHeader, got)
	}

	setReadyForTest(t, true)
	readyRec := httptest.NewRecorder()
	router.ServeHTTP(readyRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if got := readyRec.Header().Get(desktopHealthTokenHeader); got != "desktop-token" {
		t.Fatalf("ready-path %s header = %q, want desktop-token", desktopHealthTokenHeader, got)
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

	// AC-8: the compiled-in default (before this test or anything else
	// mutates the package var) must actually be "dev". Asserted against
	// prevVersion, captured above before any mutation in this test —
	// asserting post-mutation would only prove setBuildInfo doesn't clobber
	// an already-set value, not that the real default is "dev".
	if prevVersion != "dev" {
		t.Fatalf("compiled-in default Version = %q, want dev", prevVersion)
	}

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

// TestHealthHandlerFallsBackToPackageVersionWhenParamEmpty covers AC-11
// against the production wiring gap: every other test in this file passes an
// explicit routeParams{version: "..."}, so none of them would notice if the
// one-line "version: Version" field assignment that wires routeParams.version
// to the package-level Version var (main.go, in the registerRoutes call built
// by run()) were ever deleted — /health would then silently serve
// "version":"" in production. Standing up run()'s full DI graph just to
// exercise that single field is out of proportion to this change (see the
// spec's Implementation Notes). Instead, healthHandler falls back to the
// package-level Version whenever routeParams.version is unset, so the never-
// empty guarantee holds at the handler layer regardless of what the caller
// wires up.
func TestHealthHandlerFallsBackToPackageVersionWhenParamEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevVersion := Version
	t.Cleanup(func() { Version = prevVersion })
	Version = "9.9.9-fallback-test"

	router := gin.New()
	router.GET("/health", healthHandler(routeParams{}))

	for _, tc := range []struct {
		name  string
		ready bool
	}{
		{name: "ready", ready: true},
		{name: "starting", ready: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setReadyForTest(t, tc.ready)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

			var body map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["version"] != "9.9.9-fallback-test" {
				t.Fatalf("version = %v, want fallback to package-level Version %q", body["version"], "9.9.9-fallback-test")
			}
		})
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
