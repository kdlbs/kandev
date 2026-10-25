package usage

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.13
func TestTeamClaudeServicePreservesNoCapacityAfterSuccess(t *testing.T) {
	for _, pool := range []struct{ name, body string }{
		{"empty", `{"accounts":[]}`},
		{"unavailable", `{"accounts":[{"status":"throttled","quota":{"unified5h":0.1}},{"disabled":true,"status":"active","quota":{"unified5h":0.2}}]}`},
	} {
		t.Run(pool.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					_, _ = w.Write([]byte(`{"accounts":[{"status":"active","quota":{"unified5h":0.1}}]}`))
					return
				}
				_, _ = w.Write([]byte(pool.body))
			}))
			defer srv.Close()
			svc := NewUsageService()
			key := CacheKey("teamclaude", srv.URL)
			svc.Register("profile", NewTeamClaudeUsageClient(srv.URL), key)
			first, err := svc.GetUsage(t.Context(), "profile")
			if err != nil || first == nil || len(first.Windows) != 1 || first.Windows[0].UtilizationPct != 10 {
				t.Fatalf("healthy pool control: %+v %v", first, err)
			}
			svc.cache.mu.Lock()
			svc.cache.entries[key].successAt = time.Now().Add(-6 * time.Minute)
			svc.cache.mu.Unlock()
			got, err := svc.GetUsage(t.Context(), "profile")
			if err == nil || got != nil {
				t.Fatalf("authoritative pool outage masked by stale success: %+v %v", got, err)
			}
			cause := err
			for range 2 {
				limited, _, err := svc.IsPotentiallyRateLimited(t.Context(), "profile")
				if limited || !errors.Is(err, cause) {
					t.Fatalf("service policy lost pool error: limited=%v err=%v", limited, err)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("pool errors did not retain negative caching: %d calls", calls.Load())
			}
		})
	}
}

func TestTeamClaudeServiceTransportFailureStillServesStale(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"accounts":[{"status":"active","quota":{"unified5h":0.2}}]}`))
	}))
	defer srv.Close()
	svc := NewUsageService()
	key := CacheKey("teamclaude", srv.URL)
	svc.Register("profile", NewTeamClaudeUsageClient(srv.URL), key)
	first, err := svc.GetUsage(t.Context(), "profile")
	if err != nil || first == nil {
		t.Fatalf("healthy pool control: %+v %v", first, err)
	}
	svc.cache.mu.Lock()
	svc.cache.entries[key].successAt = time.Now().Add(-6 * time.Minute)
	svc.cache.mu.Unlock()
	got, err := svc.GetUsage(t.Context(), "profile")
	if err != nil || got == nil || !got.Stale || !got.FetchedAt.Equal(first.FetchedAt) || first.Stale {
		t.Fatalf("ordinary failure lost bounded stale fallback: %+v %v", got, err)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProviderUsage
	if err := json.Unmarshal(payload, &decoded); err != nil || !decoded.Stale {
		t.Fatalf("pool stale JSON: %s %v", payload, err)
	}
}
