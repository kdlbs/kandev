package usage_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
)

// mockClient implements ProviderUsageClient for testing.
type mockClient struct {
	usage *usage.ProviderUsage
	err   error
	calls int
}

func (m *mockClient) FetchUsage(_ context.Context) (*usage.ProviderUsage, error) {
	m.calls++
	return m.usage, m.err
}

func TestUsageService_GetUsage_NoClient(t *testing.T) {
	svc := usage.NewUsageService()
	got, err := svc.GetUsage(context.Background(), "unknown-profile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for unregistered profile, got %+v", got)
	}
}

func TestUsageService_GetUsage_CachesResult(t *testing.T) {
	svc := usage.NewUsageService()
	now := time.Now()
	mock := &mockClient{
		usage: &usage.ProviderUsage{
			Provider: "anthropic",
			Windows: []usage.UtilizationWindow{
				{Label: "5-hour", UtilizationPct: 50, ResetAt: now.Add(5 * time.Hour)},
			},
			FetchedAt: now,
		},
	}
	key := usage.CacheKey("anthropic", "/path/to/creds")
	svc.Register("profile-1", mock, key)

	// First call: fetches from client.
	got1, err := svc.GetUsage(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got1 == nil {
		t.Fatal("expected non-nil usage")
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 fetch call, got %d", mock.calls)
	}

	// Second call: should use cache.
	got2, err := svc.GetUsage(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 != got1 {
		t.Fatal("expected cached value to be identical pointer")
	}
	if mock.calls != 1 {
		t.Fatalf("expected still 1 fetch call after cache hit, got %d", mock.calls)
	}
}

func TestUsageService_IsPotentiallyRateLimited_NotLimited(t *testing.T) {
	svc := usage.NewUsageService()
	now := time.Now()
	mock := &mockClient{
		usage: &usage.ProviderUsage{
			Provider: "anthropic",
			Windows: []usage.UtilizationWindow{
				{Label: "5-hour", UtilizationPct: 50, ResetAt: now.Add(5 * time.Hour)},
				{Label: "7-day", UtilizationPct: 30, ResetAt: now.Add(7 * 24 * time.Hour)},
			},
			FetchedAt: now,
		},
	}
	svc.Register("profile-2", mock, usage.CacheKey("anthropic", "/creds"))

	limited, resetAt, err := svc.IsPotentiallyRateLimited(context.Background(), "profile-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limited {
		t.Fatal("expected not limited")
	}
	if !resetAt.IsZero() {
		t.Fatalf("expected zero resetAt, got %v", resetAt)
	}
}

func TestUsageService_IsPotentiallyRateLimited_Limited(t *testing.T) {
	svc := usage.NewUsageService()
	now := time.Now()
	resetTime := now.Add(3 * time.Hour)
	mock := &mockClient{
		usage: &usage.ProviderUsage{
			Provider: "anthropic",
			Windows: []usage.UtilizationWindow{
				{Label: "5-hour", UtilizationPct: 95, ResetAt: resetTime},
				{Label: "7-day", UtilizationPct: 40, ResetAt: now.Add(7 * 24 * time.Hour)},
			},
			FetchedAt: now,
		},
	}
	svc.Register("profile-3", mock, usage.CacheKey("anthropic", "/creds2"))

	limited, resetAt, err := svc.IsPotentiallyRateLimited(context.Background(), "profile-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !limited {
		t.Fatal("expected limited")
	}
	if !resetAt.Equal(resetTime) {
		t.Fatalf("expected resetAt %v, got %v", resetTime, resetAt)
	}
}

func TestUsageService_IsPotentiallyRateLimited_NoClient(t *testing.T) {
	svc := usage.NewUsageService()
	limited, resetAt, err := svc.IsPotentiallyRateLimited(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limited {
		t.Fatal("expected not limited for unregistered profile")
	}
	if !resetAt.IsZero() {
		t.Fatalf("expected zero resetAt, got %v", resetAt)
	}
}

func TestUsageService_IsPotentiallyRateLimited_ExactThreshold(t *testing.T) {
	svc := usage.NewUsageService()
	now := time.Now()
	resetTime := now.Add(time.Hour)
	mock := &mockClient{
		usage: &usage.ProviderUsage{
			Provider: "anthropic",
			Windows: []usage.UtilizationWindow{
				{Label: "5-hour", UtilizationPct: 90.0, ResetAt: resetTime},
			},
			FetchedAt: now,
		},
	}
	svc.Register("profile-4", mock, usage.CacheKey("anthropic", "/creds3"))

	limited, _, err := svc.IsPotentiallyRateLimited(context.Background(), "profile-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !limited {
		t.Fatal("expected limited at exactly 90%")
	}
}
