package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agent/usage"
)

// usageReaderStub counts calls so tests can prove which reader a route reached.
type usageReaderStub struct {
	calls   int
	entries []usage.ProfileExecutionUsage
	err     error
}

func (s *usageReaderStub) read(_ context.Context, _ string) ([]usage.ProfileExecutionUsage, error) {
	s.calls++
	return s.entries, s.err
}

func getUsage(t *testing.T, router http.Handler, path string) (int, map[string]any) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	body := map[string]any{}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	return response.Code, body
}

func TestNativeProfileUsageRoutes(t *testing.T) {
	const live, cached = "/api/v1/agent-profiles/p1/usage", "/api/v1/agent-profiles/p1/usage/cached"

	setup := func(t *testing.T, wireLive, wireCached bool) (http.Handler, *usageReaderStub, *usageReaderStub) {
		t.Helper()
		repo := newFakeSettingsRepo()
		seedProfile(repo, "p1", "agent-1", "P1", "")
		router, ctrl, _ := newSettingsHarness(t, repo, nil)
		liveReader, cachedReader := &usageReaderStub{}, &usageReaderStub{}
		if wireLive {
			ctrl.SetNativeUsageReader(liveReader.read)
		}
		if wireCached {
			ctrl.SetCachedNativeUsageReader(cachedReader.read)
		}
		return router, liveReader, cachedReader
	}

	t.Run("cached route is marked cached_only and never calls the refreshing reader", func(t *testing.T) {
		router, liveReader, cachedReader := setup(t, true, true)
		code, body := getUsage(t, router, cached)
		if code != http.StatusOK || body["cached_only"] != true || body["profile_id"] != "p1" {
			t.Fatalf("cached = %d %v, want 200 cached_only=true profile_id=p1", code, body)
		}
		if cachedReader.calls != 1 || liveReader.calls != 0 {
			t.Fatalf("calls cached=%d live=%d, want 1 and 0", cachedReader.calls, liveReader.calls)
		}
	})

	t.Run("plain usage route omits cached_only and never calls the cached reader", func(t *testing.T) {
		router, liveReader, cachedReader := setup(t, true, true)
		code, body := getUsage(t, router, live)
		if code != http.StatusOK || body["profile_id"] != "p1" {
			t.Fatalf("live = %d %v, want 200", code, body)
		}
		if _, present := body["cached_only"]; present {
			t.Fatalf("live response carries cached_only: %v", body)
		}
		if liveReader.calls != 1 || cachedReader.calls != 0 {
			t.Fatalf("calls live=%d cached=%d, want 1 and 0", liveReader.calls, cachedReader.calls)
		}
	})

	for _, path := range []string{live, cached} {
		t.Run("missing profile is 404 "+path, func(t *testing.T) {
			router, liveReader, cachedReader := setup(t, true, true)
			missing := path[:len("/api/v1/agent-profiles/")] + "missing" + path[len("/api/v1/agent-profiles/p1"):]
			if code, _ := getUsage(t, router, missing); code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", code)
			}
			if liveReader.calls+cachedReader.calls != 0 {
				t.Fatal("reader was called for a missing profile")
			}
		})
		t.Run("unwired reader is 404 "+path, func(t *testing.T) {
			router, _, _ := setup(t, false, false)
			if code, _ := getUsage(t, router, path); code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", code)
			}
		})
		t.Run("reader error is 503 "+path, func(t *testing.T) {
			router, liveReader, cachedReader := setup(t, true, true)
			liveReader.err, cachedReader.err = errors.New("boom"), errors.New("boom")
			code, body := getUsage(t, router, path)
			if code != http.StatusServiceUnavailable || body["error"] != "usage observation unavailable" {
				t.Fatalf("status = %d %v, want 503", code, body)
			}
		})
	}

	t.Run("each route needs only its own reader", func(t *testing.T) {
		router, _, _ := setup(t, true, false)
		if code, _ := getUsage(t, router, live); code != http.StatusOK {
			t.Fatalf("live status = %d, want 200", code)
		}
		if code, _ := getUsage(t, router, cached); code != http.StatusNotFound {
			t.Fatalf("cached status = %d, want 404", code)
		}
	})
}
