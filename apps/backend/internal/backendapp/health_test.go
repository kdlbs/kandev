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
// readyHandler and restores its prior value on cleanup, so tests don't leak
// state into each other or into TestMain-driven suites.
func setReadyForTest(t *testing.T, value bool) {
	t.Helper()
	prev := ready.Load()
	ready.Store(value)
	t.Cleanup(func() { ready.Store(prev) })
}

// TestHealthHandlerBodyIncludesVersionRegardlessOfReadiness covers AC-19/20
// of the 2026-08-22 liveness/readiness-split amendment
// (docs/specs/health-endpoint-version/spec.md): /health is a pure liveness
// probe, so its 200 body (status/service/mode/version, no other keys) is
// identical whether or not the `ready` flag has flipped. The old ready/
// starting two-shape split (superseded AC-1..7) now lives on readyHandler,
// covered by TestReadyHandlerBodyShapesByReadiness below.
func TestHealthHandlerBodyIncludesVersionRegardlessOfReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, readyState := range []bool{true, false} {
		setReadyForTest(t, readyState)

		router := gin.New()
		router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("ready=%v: status = %d, want %d", readyState, recorder.Code, http.StatusOK)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("ready=%v: decode body: %v", readyState, err)
		}
		if body[versionFieldKey] != "1.2.3" {
			t.Fatalf("ready=%v: version = %v, want 1.2.3", readyState, body[versionFieldKey])
		}
		if body["status"] != "ok" || body["service"] != kandevName || body["mode"] != "websocket+http" {
			t.Fatalf("ready=%v: unexpected body: %#v", readyState, body)
		}
		if len(body) != 4 {
			t.Fatalf("ready=%v: body keys = %#v, want exactly status/service/mode/version", readyState, body)
		}
	}
}

// TestReadyHandlerBodyShapesByReadiness covers AC-16..19: readyHandler now
// owns the two-shape ready/starting split that healthHandler used to have —
// 503 with status="starting" (no mode key) before ready, 200 with
// status="ok" (no mode key) once ready — always carrying version either way.
func TestReadyHandlerBodyShapesByReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, false)

	router := gin.New()
	router.GET("/ready", readyHandler(routeParams{version: "1.2.3"}))

	startingRec := httptest.NewRecorder()
	router.ServeHTTP(startingRec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if startingRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("starting status = %d, want %d", startingRec.Code, http.StatusServiceUnavailable)
	}
	var startingBody map[string]interface{}
	if err := json.Unmarshal(startingRec.Body.Bytes(), &startingBody); err != nil {
		t.Fatalf("decode starting body: %v", err)
	}
	if startingBody[versionFieldKey] != "1.2.3" {
		t.Fatalf("starting version = %v, want 1.2.3", startingBody[versionFieldKey])
	}
	if startingBody["status"] != startingStatus || startingBody["service"] != kandevName {
		t.Fatalf("unexpected starting body: %#v", startingBody)
	}
	if len(startingBody) != 3 {
		t.Fatalf("starting body keys = %#v, want exactly status/service/version", startingBody)
	}

	setReadyForTest(t, true)
	readyRec := httptest.NewRecorder()
	router.ServeHTTP(readyRec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if readyRec.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", readyRec.Code, http.StatusOK)
	}
	var readyBody map[string]interface{}
	if err := json.Unmarshal(readyRec.Body.Bytes(), &readyBody); err != nil {
		t.Fatalf("decode ready body: %v", err)
	}
	if readyBody[versionFieldKey] != "1.2.3" {
		t.Fatalf("ready version = %v, want 1.2.3", readyBody[versionFieldKey])
	}
	if readyBody["status"] != "ok" || readyBody["service"] != kandevName {
		t.Fatalf("unexpected ready body: %#v", readyBody)
	}
	if len(readyBody) != 3 {
		t.Fatalf("ready body keys = %#v, want exactly status/service/version", readyBody)
	}
}

// TestHealthHandlerDesktopTokenHeaderAlwaysSetReadyEndpointNever covers AC-21:
// the desktop health-token header is a /health-only concern, unaffected by
// the liveness/readiness split — /health sets it on every request regardless
// of readiness, and /ready never sets it at all.
func TestHealthHandlerDesktopTokenHeaderAlwaysSetReadyEndpointNever(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(desktopHealthTokenEnv, "  desktop-token  ")

	router := gin.New()
	router.GET("/health", healthHandler(routeParams{version: "1.2.3"}))
	router.GET("/ready", readyHandler(routeParams{version: "1.2.3"}))

	for _, readyState := range []bool{false, true} {
		setReadyForTest(t, readyState)

		healthRec := httptest.NewRecorder()
		router.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health", nil))
		if got := healthRec.Header().Get(desktopHealthTokenHeader); got != "desktop-token" {
			t.Fatalf("ready=%v: /health %s header = %q, want desktop-token", readyState, desktopHealthTokenHeader, got)
		}

		readyRec := httptest.NewRecorder()
		router.ServeHTTP(readyRec, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if got := readyRec.Header().Get(desktopHealthTokenHeader); got != "" {
			t.Fatalf("ready=%v: /ready %s header = %q, want absent", readyState, desktopHealthTokenHeader, got)
		}
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

	if healthBody[versionFieldKey] != infoBody.Version {
		t.Fatalf("health version = %v, system/info version = %v, want equal", healthBody[versionFieldKey], infoBody.Version)
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
		{name: startingStatus, ready: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setReadyForTest(t, tc.ready)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

			var body map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body[versionFieldKey] != "9.9.9-fallback-test" {
				t.Fatalf("version = %v, want fallback to package-level Version %q", body[versionFieldKey], "9.9.9-fallback-test")
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
	if body[versionFieldKey] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", body[versionFieldKey])
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", body["status"])
	}
}

// TestReadyHandlerAuthEnabledServesWithoutCredential covers AC-22: /ready
// must be on the unauthenticated allowlist (mirrored at the middleware layer
// by TestEnabledModeAllowlistMatrix/ready), so it never returns 401/403 to
// an unauthenticated k8s readinessProbe or e2e fixture poll even with the
// auth feature flag on.
func TestReadyHandlerAuthEnabledServesWithoutCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setReadyForTest(t, true)

	svc := newSSOTestAuthService(t)
	if _, _, err := svc.Setup(context.Background(), "admin@example.com", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatalf("Setup admin: %v", err)
	}

	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	router.GET("/ready", readyHandler(routeParams{version: "1.2.3"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no auth challenge on /ready)", recorder.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body[versionFieldKey] != "1.2.3" {
		t.Fatalf("version = %v, want 1.2.3", body[versionFieldKey])
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", body["status"])
	}
}
