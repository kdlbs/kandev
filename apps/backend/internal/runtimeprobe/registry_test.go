package runtimeprobe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestRegistry() (*Registry, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return newRegistry(clock.Now), clock
}

func ping(t *testing.T, registry *Registry, method, token string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, PingPath, registry.Handler())
	request := httptest.NewRequest(method, PingPath, http.NoBody)
	if token != "" {
		request.Header.Set(HeaderName, token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestMatchedTokenAnswers204AndMarksTheProbeReached(t *testing.T) {
	registry, _ := newTestRegistry()
	probe := registry.Begin("environment-a")
	if probe.Reached() {
		t.Fatal("probe reported reached before any request")
	}
	if code := ping(t, registry, http.MethodGet, probe.Token()); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	if !probe.Reached() {
		t.Fatal("a matched request must count as success even if its reply is lost")
	}
}

func TestTokenIsValidForBothAttempts(t *testing.T) {
	registry, _ := newTestRegistry()
	probe := registry.Begin("environment-a")
	for attempt := 0; attempt < 2; attempt++ {
		if code := ping(t, registry, http.MethodGet, probe.Token()); code != http.StatusNoContent {
			t.Fatalf("attempt %d status = %d, want 204", attempt+1, code)
		}
	}
}

func TestUnknownMissingExpiredAndEndedTokensAreRefused(t *testing.T) {
	registry, clock := newTestRegistry()
	probe := registry.Begin("environment-a")
	other := registry.Begin("environment-b")
	other.End()
	ended := registry.Begin("environment-c")
	endedToken := ended.Token()
	ended.End()

	cases := map[string]string{"missing": "", "wrong": "not-a-token", "other environment, ended": other.Token(), "ended": endedToken}
	for name, token := range cases {
		if code := ping(t, registry, http.MethodGet, token); code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401", name, code)
		}
	}
	clock.now = clock.now.Add(TokenLifetime + time.Second)
	if code := ping(t, registry, http.MethodGet, probe.Token()); code != http.StatusUnauthorized {
		t.Fatalf("expired: status = %d, want 401", code)
	}
	if probe.Reached() {
		t.Fatal("a refused request marked the probe reached")
	}
}

func TestATokenOfAnotherEnvironmentNeverMarksThisProbe(t *testing.T) {
	registry, _ := newTestRegistry()
	first := registry.Begin("environment-a")
	second := registry.Begin("environment-b")
	if code := ping(t, registry, http.MethodGet, second.Token()); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	if first.Reached() || !second.Reached() {
		t.Fatalf("reached: first=%v second=%v, want only the second", first.Reached(), second.Reached())
	}
}

func TestEndDeletesTheTokenAndIsIdempotent(t *testing.T) {
	registry, _ := newTestRegistry()
	probe := registry.Begin("environment-a")
	probe.End()
	probe.End()
	if code := ping(t, registry, http.MethodGet, probe.Token()); code != http.StatusUnauthorized {
		t.Fatalf("status after End = %d, want 401", code)
	}
	if registry.Active() != 0 {
		t.Fatalf("active = %d, want 0", registry.Active())
	}
}

func TestTokensAreRandomAndPerProbe(t *testing.T) {
	registry, _ := newTestRegistry()
	a, b := registry.Begin("environment-a"), registry.Begin("environment-a")
	if a.Token() == "" || a.Token() == b.Token() {
		t.Fatalf("tokens %q and %q must be non-empty and distinct", a.Token(), b.Token())
	}
}
