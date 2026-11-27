package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func localUsageClient(t *testing.T, provider, endpoint string) ProviderUsageClient {
	t.Helper()
	if provider == "claude" {
		c := NewClaudeUsageClientWithPath(writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil))
		c.usageURL, c.refreshURL = endpoint, endpoint+"/refresh"
		return c
	}
	c := NewCodexUsageClientWithPath(writeCodexAuth(t, t.TempDir(), true))
	c.usageURL, c.refreshURL = endpoint, endpoint+"/refresh"
	return c
}

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.9
func TestProviderFetchUsageServiceStatusAndStale(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/" {
					t.Error("429 unexpectedly attempted token refresh")
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("provider busy"))
			}))
			defer srv.Close()
			client := localUsageClient(t, provider, srv.URL)
			svc := NewUsageService()
			key := CacheKey(provider, "temporary credential")
			svc.Register("one", client, key)
			svc.Register("same-credential", client, key)
			for _, profile := range []string{"one", "same-credential"} {
				got, err := svc.GetUsage(t.Context(), profile)
				var status statusCodeHint
				var hint retryAfterHint
				if got != nil || !errors.As(err, &status) || status.StatusCode() != 429 || !errors.As(err, &hint) || hint.RetryAfter() != 2*time.Minute {
					t.Fatalf("public service lost typed status/hint: %+v %v", got, err)
				}
				if !isRateLimited(fmt.Errorf("caller: %w", err)) {
					t.Fatal("wrapped public failure lost classification")
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("registered credential did not share backoff: %d", calls.Load())
			}
			reading := &ProviderUsage{Provider: provider, FetchedAt: time.Now().Add(-6 * time.Minute), Windows: []UtilizationWindow{{Label: "5-hour", UtilizationPct: 95, ResetAt: time.Now().Add(time.Hour)}}}
			svc.cache.storeSuccess(key, reading)
			svc.cache.entries[key].successAt = reading.FetchedAt
			got, err := svc.GetUsage(t.Context(), "one")
			if err != nil || got == nil || !got.Stale || !got.FetchedAt.Equal(reading.FetchedAt) || reading.Stale {
				t.Fatalf("public stale result: %+v %v", got, err)
			}
			payload, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ProviderUsage
			if err := json.Unmarshal(payload, &decoded); err != nil || !decoded.Stale || !decoded.FetchedAt.Equal(reading.FetchedAt) {
				t.Fatalf("stale JSON contract lost: %s %v", payload, err)
			}
			limited, reset, err := svc.IsPotentiallyRateLimited(t.Context(), "same-credential")
			if err != nil || !limited || !reset.Equal(reading.Windows[0].ResetAt) || calls.Load() != 2 {
				t.Fatalf("service policy changed real preserved windows: limited=%v reset=%v calls=%d err=%v", limited, reset, calls.Load(), err)
			}
		})
	}
}

func TestProviderFetchUsageCancellationThroughService(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			started := make(chan struct{})
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-r.Context().Done()
					return
				}
				if provider == "claude" {
					_, _ = w.Write([]byte(`{"five_hour":{"utilization":23}}`))
				} else {
					_, _ = w.Write([]byte(codexUsageBody))
				}
			}))
			defer srv.Close()
			svc := NewUsageService()
			svc.Register("profile", localUsageClient(t, provider, srv.URL), CacheKey(provider, "temporary"))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := svc.GetUsage(ctx, "profile"); done <- err }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("local HTTP request did not start")
			}
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("FetchUsage cause lost through service: %v", err)
			}
			got, err := svc.GetUsage(t.Context(), "profile")
			if err != nil || got == nil || got.Stale || calls.Load() != 2 {
				t.Fatalf("cancelled HTTP poisoned service: %+v %v calls=%d", got, err, calls.Load())
			}
		})
	}
}

func TestCodexRefreshRetryReturnsFinalStatus(t *testing.T) {
	var calls, refreshes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/refresh" {
			refreshes.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"refreshed"}`))
			return
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer refreshed" || r.Header.Get("chatgpt-account-id") != "acct-123" {
			t.Error("refreshed request lost credentials/account")
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	client := localUsageClient(t, "codex", srv.URL)
	_, err := client.FetchUsage(t.Context())
	var status statusCodeHint
	var hint retryAfterHint
	if !errors.As(err, &status) || status.StatusCode() != 429 || !errors.As(err, &hint) || hint.RetryAfter() != time.Minute || calls.Load() != 2 || refreshes.Load() != 1 {
		t.Fatalf("final refreshed status lost: %v calls=%d refreshes=%d", err, calls.Load(), refreshes.Load())
	}
}

func TestClaudeUnauthorizedDoesNotRefreshValidToken(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/" {
			t.Error("valid Claude token was refreshed on 401")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := localUsageClient(t, "claude", srv.URL).FetchUsage(t.Context())
	var status statusCodeHint
	if !errors.As(err, &status) || status.StatusCode() != 401 || calls.Load() != 1 {
		t.Fatalf("Claude 401 behavior changed: %v calls=%d", err, calls.Load())
	}
}

func TestProviderFetchUsageRetryHeaderBoundaries(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			for _, tc := range []struct {
				name, value string
				want        time.Duration
			}{
				{"absent", "", 0}, {"delta", "60", time.Minute}, {"overflow", "99999999999999999999", 15 * time.Minute},
				{"negative", "-99999999999999999999", 0}, {"malformed", "99999999999999999999x", 0},
				{"date cap", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat), 15 * time.Minute},
			} {
				t.Run(tc.name, func(t *testing.T) {
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Retry-After", tc.value)
						w.WriteHeader(http.StatusServiceUnavailable)
					}))
					defer srv.Close()
					got, err := localUsageClient(t, provider, srv.URL).FetchUsage(t.Context())
					var status statusCodeHint
					var hint retryAfterHint
					if got != nil || !errors.As(err, &status) || status.StatusCode() != 503 || !errors.As(err, &hint) || hint.RetryAfter() != tc.want || isRateLimited(err) {
						t.Fatalf("public non-429 status/hint: got=%+v err=%v", got, err)
					}
				})
			}
		})
	}
}
