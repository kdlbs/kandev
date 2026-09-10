package webapp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestRuntimeServesEntryWithSecurityHeadersAndNoCookies(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": staticManifestYAML,
		"ui/index.html": "<!doctype html><html><body>safe</body></html>",
	})
	pkg, err := ValidatePackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidatePackage: %v", err)
	}
	artifacts, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}
	artifact, err := artifacts.Put(pkg)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	manager := NewTokenManager(nil)
	token, err := manager.Issue(CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", ReleaseID: "release-1", WebAppKey: "main", Placement: "task-canvas", Artifact: artifact, Entry: "ui/index.html",
	}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	runtime := NewRuntime(manager, artifacts, nil, []string{"http://127.0.0.1:38429", "tauri://localhost", "http://tauri.localhost"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "null")
	req.AddCookie(&http.Cookie{Name: "kandev_session", Value: "must-not-be-used"})
	response := httptest.NewRecorder()
	runtime.Serve(response, req, token, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "safe") {
		t.Fatalf("body = %q", response.Body.String())
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox allow-scripts allow-forms") || !strings.Contains(got, "form-action 'none'") || !strings.Contains(got, "frame-ancestors http://127.0.0.1:38429") {
		t.Fatalf("CSP = %q", got)
	}
	for key, want := range map[string]string{
		"Cache-Control":                "no-store",
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "cross-origin",
		"Access-Control-Allow-Origin":  "null",
	} {
		if got := response.Header().Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := response.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("runtime set a cookie: %q", got)
	}
}

func TestRuntimeResolvesNestedEntryAssetsAndKeepsProtocolRoot(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": staticManifestYAML,
		"ui/index.html": `<script src="./app.js"></script><link rel="stylesheet" href="./app.css">`,
		"ui/app.js":     `fetch("./_kandev/v1/context")`,
		"ui/app.css":    `body { color: red; }`,
	})
	pkg, err := ValidatePackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidatePackage: %v", err)
	}
	artifacts, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}
	artifact, err := artifacts.Put(pkg)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	manager := NewTokenManager(nil)
	binding := CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", ReleaseID: "release-1", WebAppKey: "main",
		Placement: "task-canvas", Artifact: artifact, Entry: "ui/index.html",
	}
	token, err := manager.Issue(binding, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	runtime := NewRuntime(manager, artifacts, nil, nil)
	runtime.SetProtocolHandler(func(w http.ResponseWriter, _ *http.Request, _ string, _ CapabilityBinding, protocolPath string) {
		if protocolPath != "v1/context" {
			t.Errorf("protocol path = %q, want v1/context", protocolPath)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"protocol":true}`))
	})

	for requestPath, want := range map[string]string{
		"app.js":  `fetch("./_kandev/v1/context")`,
		"app.css": `body { color: red; }`,
	} {
		response := httptest.NewRecorder()
		runtime.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), token, requestPath)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("%s response = %d %q, want 200 %q", requestPath, response.Code, response.Body.String(), want)
		}
	}

	protocolResponse := httptest.NewRecorder()
	runtime.Serve(protocolResponse, httptest.NewRequest(http.MethodGet, "/", nil), token, "_kandev/v1/context")
	if protocolResponse.Code != http.StatusOK || protocolResponse.Body.String() != `{"protocol":true}` {
		t.Fatalf("protocol response = %d %q", protocolResponse.Code, protocolResponse.Body.String())
	}
}

func TestRuntimeRejectsStaleCapabilityBeforeReadingArtifact(t *testing.T) {
	manager := NewTokenManager(nil)
	token, err := manager.Issue(CapabilityBinding{UserID: "u", InstanceID: "i", ReleaseID: "r", WebAppKey: "main", Artifact: Artifact{Digest: strings.Repeat("c", 64), RelativePath: "releases/" + strings.Repeat("c", 64)}, Entry: "ui/index.html"}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	runtime := NewRuntime(manager, nil, func(_ context.Context, _ CapabilityBinding) error { return ErrRuntimeTokenStale }, nil)
	response := httptest.NewRecorder()
	runtime.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), token, "ui/index.html")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "runtime_token_stale") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if body, _ := io.ReadAll(response.Body); len(body) == 0 {
		t.Fatal("stale token response has no safe error")
	}
}

func TestRuntimeRevalidatesBindingAfterAuthorityRevocation(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": staticManifestYAML,
		"ui/index.html": "entry",
		"ui/app.js":     "script",
	})
	pkg, err := ValidatePackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidatePackage: %v", err)
	}
	artifacts, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}
	artifact, err := artifacts.Put(pkg)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	manager := NewTokenManager(nil)
	token, err := manager.Issue(CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", ReleaseID: "release-1", WebAppKey: "main",
		Placement: "task-canvas", Artifact: artifact, Entry: "ui/index.html",
	}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	revoked := false
	runtime := NewRuntime(manager, artifacts, func(_ context.Context, _ CapabilityBinding) error {
		if revoked {
			return ErrRuntimeTokenStale
		}
		return nil
	}, nil)
	first := httptest.NewRecorder()
	runtime.Serve(first, httptest.NewRequest(http.MethodGet, "/", nil), token, "")
	if first.Code != http.StatusOK {
		t.Fatalf("initial entry response = %d %q, want 200", first.Code, first.Body.String())
	}
	revoked = true
	second := httptest.NewRecorder()
	runtime.Serve(second, httptest.NewRequest(http.MethodGet, "/", nil), token, "app.js")
	if second.Code != http.StatusUnauthorized || !strings.Contains(second.Body.String(), "runtime_token_stale") {
		t.Fatalf("post-revocation asset response = %d %q, want stale-token denial", second.Code, second.Body.String())
	}
}

func TestFrameAncestorsForConfigIncludesExactBrowserLauncherOrigins(t *testing.T) {
	origins, err := FrameAncestorsForConfig(48123, 48124, "http://localhost:48124")
	if err != nil {
		t.Fatalf("FrameAncestorsForConfig() unexpected error: %v", err)
	}
	want := []string{
		"http://localhost:48123",
		"http://127.0.0.1:48123",
		"http://localhost:48124",
		"tauri://localhost",
		"http://tauri.localhost",
	}
	for _, origin := range want {
		if !containsString(origins, origin) {
			t.Fatalf("frame origins = %v, missing %q", origins, origin)
		}
	}
	for _, origin := range origins {
		if strings.Contains(origin, "*") {
			t.Fatalf("frame origin %q contains a wildcard", origin)
		}
	}
}
