package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// ---------------------------------------------------------------------------
// isRateLimitError
// ---------------------------------------------------------------------------

func TestIsRateLimitError_Positive(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"rate limit lowercase", "rate limit exceeded"},
		{"rate_limit underscore", "error: rate_limit"},
		{"429 status code", "429 Too Many Requests"},
		{"too many requests", "Too Many Requests from this IP"},
		{"quota exceeded", "Quota exceeded for today"},
		{"mixed case", "RATE LIMIT hit for model"},
		{"429 embedded", "received HTTP/429 from upstream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !service.IsRateLimitErrorForTest(tc.msg) {
				t.Errorf("isRateLimitError(%q) = false, want true", tc.msg)
			}
		})
	}
}

func TestIsRateLimitError_Negative(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"network timeout", "connection timed out"},
		{"not found", "resource not found"},
		{"empty", ""},
		{"server error", "internal server error 500"},
		{"partial keyword", "unrated content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if service.IsRateLimitErrorForTest(tc.msg) {
				t.Errorf("isRateLimitError(%q) = true, want false", tc.msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseRateLimitResetTime
// ---------------------------------------------------------------------------

func TestParseRateLimitResetTime_RetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		msg     string
		wantSec int64 // expected seconds from now (before buffer)
	}{
		{"retry-after colon", "Retry-After: 3600", 3600},
		{"retry-after space", "retry after 120", 120},
		{"retry-after dash", "retry-after:60", 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseRateLimitResetTimeForTest(tc.msg, now)
			if got == nil {
				t.Fatalf("parseRateLimitResetTime(%q) = nil, want non-nil", tc.msg)
			}
			// Expected = now + wantSec + 30s buffer
			wantTime := now.Add(time.Duration(tc.wantSec)*time.Second + 30*time.Second)
			if !got.Equal(wantTime) {
				t.Errorf("got %v, want %v", *got, wantTime)
			}
		})
	}
}

func TestParseRateLimitResetTime_TryAgainIn(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		msg      string
		wantTime time.Time
	}{
		{
			"minutes",
			"Please try again in 5 minutes",
			now.Add(5*time.Minute + 30*time.Second),
		},
		{
			"seconds",
			"try again in 30 seconds",
			now.Add(30*time.Second + 30*time.Second),
		},
		{
			"hours",
			"try again in 2 hours",
			now.Add(2*time.Hour + 30*time.Second),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseRateLimitResetTimeForTest(tc.msg, now)
			if got == nil {
				t.Fatalf("parseRateLimitResetTime(%q) = nil, want non-nil", tc.msg)
			}
			if !got.Equal(tc.wantTime) {
				t.Errorf("got %v, want %v", *got, tc.wantTime)
			}
		})
	}
}

func TestParseRateLimitResetTime_ResetsAt(t *testing.T) {
	// Base: 3:45 AM UTC
	now := time.Date(2026, 1, 15, 3, 45, 0, 0, time.UTC)

	cases := []struct {
		name     string
		msg      string
		wantTime time.Time
	}{
		{
			"12-hour AM future",
			"rate limit resets at 4:00 AM",
			time.Date(2026, 1, 15, 4, 0, 30, 0, time.UTC),
		},
		{
			"24-hour future",
			"resets at 04:30",
			time.Date(2026, 1, 15, 4, 30, 30, 0, time.UTC),
		},
		{
			"already past — next day",
			"resets at 3:00 AM",
			time.Date(2026, 1, 16, 3, 0, 30, 0, time.UTC),
		},
		{
			"midnight rollover PM",
			"reset at 11:59 PM",
			time.Date(2026, 1, 15, 23, 59, 30, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseRateLimitResetTimeForTest(tc.msg, now)
			if got == nil {
				t.Fatalf("parseRateLimitResetTime(%q) = nil, want non-nil", tc.msg)
			}
			if !got.Equal(tc.wantTime) {
				t.Errorf("got %v, want %v", *got, tc.wantTime)
			}
		})
	}
}

func TestParseRateLimitResetTime_UnixTimestamp(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		msg  string
		unix int64
	}{
		{"seconds", `rate limit exceeded reset_time: 1752624000`, 1752624000},
		{"milliseconds", `rate limit exceeded reset_time: 1752624000000`, 1752624000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseRateLimitResetTimeForTest(tc.msg, now)
			if got == nil {
				t.Fatalf("parseRateLimitResetTime(%q) = nil, want non-nil", tc.msg)
			}
			want := time.Unix(tc.unix, 0).Add(30 * time.Second)
			if !got.Equal(want) {
				t.Errorf("got %v, want %v", *got, want)
			}
		})
	}
}

func TestParseRateLimitResetTime_Unparseable(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		msg  string
	}{
		{"no reset info", "rate limit exceeded"},
		{"empty", ""},
		{"generic timeout", "connection timed out after 30s"},
		{"rate limit no time", "429 rate_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseRateLimitResetTimeForTest(tc.msg, now)
			if got != nil {
				t.Errorf("parseRateLimitResetTime(%q) = %v, want nil", tc.msg, *got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleRunFailure integration
// ---------------------------------------------------------------------------

func TestHandleRunFailure_RateLimitParsed_UsesResetTime(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k1"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	before := time.Now().UTC()
	// Error with Retry-After: 3600 (1 hour)
	rlErr := errors.New("429 Too Many Requests, Retry-After: 3600")
	if err := svc.HandleRunFailure(ctx, run, rlErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	req := reqs[0]
	if req.Status != service.RunStatusQueued {
		t.Errorf("status = %q, want %q", req.Status, service.RunStatusQueued)
	}
	if req.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", req.RetryCount)
	}
	if req.ScheduledRetryAt == nil {
		t.Fatal("scheduled_retry_at is nil")
	}

	// Should be approximately now + 3600s + 30s (within a 5s tolerance).
	expectedLow := before.Add(3600*time.Second + 25*time.Second)
	expectedHigh := before.Add(3600*time.Second + 35*time.Second)
	if req.ScheduledRetryAt.Before(expectedLow) || req.ScheduledRetryAt.After(expectedHigh) {
		t.Errorf("scheduled_retry_at = %v, want ~%v", *req.ScheduledRetryAt, expectedLow)
	}
}

func TestHandleRunFailure_RateLimitNoParseable_UsesBackoff(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k2"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	before := time.Now().UTC()
	// Rate-limit error with NO parseable reset time → falls back to backoff.
	rlErr := errors.New("rate limit exceeded, please wait")
	if err := svc.HandleRunFailure(ctx, run, rlErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	req := reqs[0]
	if req.Status != service.RunStatusQueued {
		t.Errorf("status = %q, want %q", req.Status, service.RunStatusQueued)
	}
	if req.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", req.RetryCount)
	}
	if req.ScheduledRetryAt == nil {
		t.Fatal("scheduled_retry_at is nil")
	}

	// Backoff[0] = 2 minutes + up to 30s jitter. The scheduled time should be
	// well below the rate-limit window (3600s), confirming the backoff path.
	maxBackoff := before.Add(3 * time.Minute) // 2m base + 25% jitter ceiling
	if req.ScheduledRetryAt.After(maxBackoff) {
		t.Errorf("scheduled_retry_at %v looks like rate-limit path, want backoff (<%v)",
			*req.ScheduledRetryAt, maxBackoff)
	}
}

func TestHandleRunFailure_NonRateLimit_UsesBackoff(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k3"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	// Generic network error — no rate-limit keywords.
	netErr := errors.New("connection timed out")
	if err := svc.HandleRunFailure(ctx, run, netErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	reqs, _ := svc.ListRuns(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	if reqs[0].Status != service.RunStatusQueued {
		t.Errorf("status = %q, want queued", reqs[0].Status)
	}
	if reqs[0].RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", reqs[0].RetryCount)
	}
}
