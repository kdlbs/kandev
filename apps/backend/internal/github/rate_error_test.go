package github

import (
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-001.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.3
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.4
func TestClassifyGitHubResponse(t *testing.T) {
	now := time.Date(2026, 8, 29, 5, 18, 9, 0, time.UTC)
	reset := now.Add(time.Hour)
	tests := []struct {
		name       string
		status     int
		body       string
		headers    http.Header
		wantKind   FailureKind
		wantSource RetrySource
		wantRetry  time.Time
	}{
		{
			name: "primary zero remaining", status: http.StatusForbidden,
			body: `{"message":"API rate limit exceeded"}`,
			headers: http.Header{
				"X-Ratelimit-Remaining": {"0"},
				"X-Ratelimit-Reset":     {strconv.FormatInt(reset.Unix(), 10)},
			},
			wantKind: FailurePrimaryRateLimit, wantSource: RetrySourcePrimaryReset, wantRetry: reset,
		},
		{
			name: "generic forbidden with zero remaining is primary", status: http.StatusForbidden,
			body: `{"message":"Forbidden"}`,
			headers: http.Header{
				"X-Ratelimit-Remaining": {"0"},
				"X-Ratelimit-Reset":     {strconv.FormatInt(reset.Unix(), 10)},
			},
			wantKind: FailurePrimaryRateLimit, wantSource: RetrySourcePrimaryReset, wantRetry: reset,
		},
		{
			name: "live incident full primary plus refusal", status: http.StatusForbidden,
			body: `{"message":"API rate limit exceeded for user ID 79718216"}`,
			headers: http.Header{
				"X-Ratelimit-Limit":     {"5000"},
				"X-Ratelimit-Remaining": {"5000"},
				"X-Ratelimit-Reset":     {strconv.FormatInt(reset.Unix(), 10)},
				"Retry-After":           {"90"},
			},
			wantKind: FailureSecondaryRateLimit, wantSource: RetrySourceRetryAfter,
			wantRetry: now.Add(90 * time.Second),
		},
		{
			name: "secondary 429 fallback", status: http.StatusTooManyRequests,
			wantKind: FailureSecondaryRateLimit, wantSource: RetrySourceConservativeFallback,
			wantRetry: now.Add(time.Minute),
		},
		{
			name: "invalid credentials", status: http.StatusUnauthorized,
			body: `{"message":"Bad credentials"}`, wantKind: FailureInvalidCredentials,
		},
		{
			name: "generic forbidden suspends on invalid credentials", status: http.StatusForbidden,
			body: `{"message":"Forbidden"}`, wantKind: FailureInvalidCredentials,
		},
		{
			name: "missing resource", status: http.StatusNotFound,
			body: `{"message":"Not Found"}`, wantKind: FailureMissingResource,
		},
		{
			name: "provider transient", status: http.StatusServiceUnavailable,
			wantKind: FailureTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: tt.headers}
			got := classifyGitHubResponse(resp, "/repos/kdlbs/kandev", []byte(tt.body), now)
			if got.Kind != tt.wantKind || got.RetrySource != tt.wantSource {
				t.Fatalf("classification = (%s, %s), want (%s, %s)",
					got.Kind, got.RetrySource, tt.wantKind, tt.wantSource)
			}
			if !got.RetryAt.Equal(tt.wantRetry) {
				t.Fatalf("retry_at = %s, want %s", got.RetryAt, tt.wantRetry)
			}
		})
	}
}

func TestClassifyGitHubResponseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 29, 5, 18, 9, 0, time.UTC)
	retry := now.Add(3 * time.Minute)
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{
		"Retry-After": {retry.Format(http.TimeFormat)},
	}}
	got := classifyGitHubResponse(resp, "/user", []byte(`{"message":"secondary rate limit"}`), now)
	if got.RetrySource != RetrySourceRetryAfter || !got.RetryAt.Equal(retry) {
		t.Fatalf("classification = %+v", got)
	}
}

func TestFailureKindOfClassifiesNetworkTransportAsTransient(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://api.github.com/user", Err: syscall.ECONNREFUSED}
	if got := FailureKindOf(err); got != FailureTransient {
		t.Fatalf("failure kind = %q, want %q", got, FailureTransient)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-004.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-004.2
func TestOperationRateLimitFromAdmissionError(t *testing.T) {
	now := time.Date(2026, 8, 30, 11, 18, 0, 0, time.UTC)
	retryAt := now.Add(90 * time.Second)
	details, ok := OperationRateLimitFromError(&AdmissionDeferredError{
		Resource: ResourceGraphQL, Reason: rateLimitBlockPrimaryReserve,
		RetryAt: retryAt, RetrySource: RetrySourcePrimaryReset,
	}, now)
	if !ok {
		t.Fatal("primary reserve admission error was not identified as rate limited")
	}
	if details.Kind != OperationRateLimitInteractiveReserve || details.Resource != ResourceGraphQL {
		t.Fatalf("details = %+v", details)
	}
	if details.RetryAt == nil || !details.RetryAt.Equal(retryAt) {
		t.Fatalf("retry_at = %v, want %s", details.RetryAt, retryAt)
	}
	if details.RetryAfterSeconds != 90 || details.Source != "rate_limit_reset" {
		t.Fatalf("retry details = %+v", details)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-001.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.4
func TestClassifyGitHubResponseGenericForbiddenWithRetryAfterIsSecondary(t *testing.T) {
	now := time.Date(2026, 8, 29, 5, 18, 9, 0, time.UTC)
	tests := []struct {
		name        string
		remaining   string
		retryHeader string
		otherBody   bool
	}{
		{
			name:        "generic forbidden with retry-after and positive remaining",
			remaining:   "5000",
			retryHeader: "90",
		},
		{
			name:        "generic forbidden with retry-after and unknown remaining",
			retryHeader: "90",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.remaining != "" {
				headers.Set("X-Ratelimit-Remaining", tt.remaining)
			}
			headers.Set("Retry-After", tt.retryHeader)
			resp := &http.Response{StatusCode: http.StatusForbidden, Header: headers}
			got := classifyGitHubResponse(resp, "/user", []byte(`{"message":"Forbidden"}`), now)
			if got.Kind != FailureSecondaryRateLimit {
				t.Fatalf("kind = %s, want %s", got.Kind, FailureSecondaryRateLimit)
			}
			if got.RetrySource != RetrySourceRetryAfter || !got.RetryAt.Equal(now.Add(90*time.Second)) {
				t.Fatalf("retry boundary = (%s, %s), want (%s, %s)",
					got.RetryAt.Format(time.RFC3339), got.RetrySource,
					now.Add(90*time.Second).Format(time.RFC3339), RetrySourceRetryAfter)
			}
		})
	}
}
