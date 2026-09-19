package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sprites "github.com/superfly/sprites-go"
)

type testNetworkError struct {
	temporary bool
}

func (e testNetworkError) Error() string   { return "test network error" }
func (e testNetworkError) Timeout() bool   { return false }
func (e testNetworkError) Temporary() bool { return e.temporary }

func TestIsTransientSpriteErrorClassifiesTemporaryNetworkErrors(t *testing.T) {
	if !isTransientSpriteError(fmt.Errorf("lookup: %w", testNetworkError{temporary: true})) {
		t.Fatal("temporary network error should be retryable")
	}
	if isTransientSpriteError(testNetworkError{}) {
		t.Fatal("permanent network error should not be retryable")
	}
}

func TestGetOrCreateSpriteRetriesTransientGet(t *testing.T) {
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		if getCalls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"name":"kandev-pr-2115","status":"created"}`))
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL))
	t.Cleanup(func() { _ = client.Close() })

	sprite, err := getOrCreateSprite(t.Context(), client, "kandev-pr-2115")
	if err != nil {
		t.Fatalf("getOrCreateSprite() error = %v", err)
	}
	if sprite.Name() != "kandev-pr-2115" {
		t.Errorf("sprite.Name() = %q", sprite.Name())
	}
	if getCalls != 2 {
		t.Errorf("get calls = %d, want 2", getCalls)
	}
}

func TestGetOrCreateSpriteReconcilesTransientCreateFailure(t *testing.T) {
	getCalls := 0
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getCalls++
			if getCalls == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"name":"kandev-pr-2115","status":"created"}`))
		case http.MethodPost:
			createCalls++
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected %s request", r.Method)
		}
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL))
	t.Cleanup(func() { _ = client.Close() })

	sprite, err := getOrCreateSprite(t.Context(), client, "kandev-pr-2115")
	if err != nil {
		t.Fatalf("getOrCreateSprite() error = %v", err)
	}
	if sprite.Name() != "kandev-pr-2115" {
		t.Errorf("sprite.Name() = %q", sprite.Name())
	}
	if createCalls != 1 {
		t.Errorf("create calls = %d, want 1", createCalls)
	}
	if getCalls != 2 {
		t.Errorf("get calls = %d, want 2", getCalls)
	}
}

func TestGetOrCreateSpriteDoesNotRetryPermanentErrors(t *testing.T) {
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL))
	t.Cleanup(func() { _ = client.Close() })

	_, err := getOrCreateSprite(t.Context(), client, "kandev-pr-2115")
	if err == nil {
		t.Fatal("getOrCreateSprite() error = nil, want permanent error")
	}
	if getCalls != 1 {
		t.Errorf("get calls = %d, want 1", getCalls)
	}
}

func TestGetOrCreateSpriteReturnsFinalTransientErrorAfterRetryBudget(t *testing.T) {
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL))
	t.Cleanup(func() { _ = client.Close() })

	_, err := getOrCreateSprite(t.Context(), client, "kandev-pr-2115")
	if err == nil {
		t.Fatal("getOrCreateSprite() error = nil, want transient error")
	}
	if !strings.Contains(err.Error(), "get sprite") {
		t.Errorf("getOrCreateSprite() error = %v, want get sprite error", err)
	}
	if getCalls != spriteControlRetries {
		t.Errorf("get calls = %d, want %d", getCalls, spriteControlRetries)
	}
}

func TestBuildExtractScript(t *testing.T) {
	script := buildExtractScript(12345)

	if !strings.Contains(script, "--backend-port 12345") {
		t.Errorf("expected --backend-port 12345 in script, got:\n%s", script)
	}
	if !strings.Contains(script, "rm -rf /data") {
		t.Errorf("expected rm -rf /data in script")
	}
	if !strings.Contains(script, "KANDEV_MOCK_AGENT=true") {
		t.Errorf("expected KANDEV_MOCK_AGENT=true in script")
	}
	if strings.Contains(script, "KANDEV_MOCK_AGENT=only") {
		t.Errorf("preview must retain the built-in agent catalogue")
	}
	if !strings.Contains(script, "KANDEV_WEB_DIST_DIR=/app/apps/web/dist") {
		t.Errorf("expected KANDEV_WEB_DIST_DIR to point at packaged Vite dist")
	}
	if !strings.Contains(script, "KANDEV_WEB_TITLE_PREFIX=Preview") {
		t.Errorf("expected preview title prefix in script")
	}
	if !strings.Contains(script, "ln -sf /app/apps/backend/bin/kandev      /usr/local/bin/kandev") {
		t.Errorf("expected native kandev symlink in script")
	}
	if !strings.Contains(script, "exec kandev start") {
		t.Errorf("expected script to launch through kandev start")
	}
	if !strings.Contains(script, "--headless") {
		t.Errorf("expected headless CLI launch")
	}
	if strings.Contains(script, "nohup node") {
		t.Errorf("script should not start web outside the CLI supervisor")
	}
	if strings.Contains(script, ".next") {
		t.Errorf("script should not refer to Next.js build output")
	}
	if strings.Contains(script, "/app/apps/backend/bin/kandev >") {
		t.Errorf("script should not launch the backend binary directly")
	}
}

// TestKandevReadyURLPollsReadyNotHealth is the regression test for Review
// round 3 finding R3-1: waitForKandev used to poll /health, which this
// branch redefined as an unconditional-200 liveness probe served by the
// bootstrap handler the instant the socket binds. That made preview-env CI
// (.github/workflows/preview-env.yml) declare the deploy ready — and post
// the PR comment with the URL — while every non-/health path, including the
// whole app, was still returning the bootstrap handler's 503 "starting".
//
// Expected pre-fix failure: kandevReadyURL did not exist (waitForKandev
// built the URL inline as ".../health"), so this fails to compile against
// the pre-fix code, and once inlined the URL would end in /health, not
// /ready.
func TestKandevReadyURLPollsReadyNotHealth(t *testing.T) {
	got := kandevReadyURL(4173)
	want := "http://localhost:4173/ready"
	if got != want {
		t.Fatalf("kandevReadyURL(4173) = %q, want %q", got, want)
	}
	if strings.Contains(got, "/health") {
		t.Fatalf("kandevReadyURL must not poll /health — the bootstrap handler answers "+
			"200 there before the real router exists, so preview-env would declare "+
			"success while every other path still 503s: %q", got)
	}
}
