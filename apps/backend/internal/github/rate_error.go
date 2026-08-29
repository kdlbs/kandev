package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FailureKind identifies the remediation required for a GitHub failure.
type FailureKind string

const (
	FailureUnknown            FailureKind = "unknown"
	FailurePrimaryRateLimit   FailureKind = "primary_rate_limit"
	FailureSecondaryRateLimit FailureKind = "secondary_rate_limit"
	FailureInvalidCredentials FailureKind = "invalid_credentials"
	FailureMissingResource    FailureKind = "missing_resource"
	FailureTransient          FailureKind = "transient"
)

// RetrySource identifies who supplied the retry boundary.
type RetrySource string

const (
	RetrySourceNone                 RetrySource = ""
	RetrySourceRetryAfter           RetrySource = "retry_after"
	RetrySourcePrimaryReset         RetrySource = "primary_reset"
	RetrySourceConservativeFallback RetrySource = "conservative_fallback"
)

const secondaryFallbackDelay = time.Minute

type githubFailure struct {
	Kind        FailureKind
	Resource    Resource
	RetryAt     time.Time
	RetrySource RetrySource
	Snapshot    *RateSnapshot
}

func classifyGitHubResponse(resp *http.Response, endpoint string, body []byte, now time.Time) githubFailure {
	resource := resourceForEndpoint(endpoint)
	result := githubFailure{Kind: FailureUnknown, Resource: resource}
	if resp == nil {
		return result
	}
	if snap, ok := parseRateHeadersAt(resp, resource, now); ok {
		result.Resource = snap.Resource
		result.Snapshot = &snap
	}

	result.Kind = classifyFailureKind(resp.StatusCode, string(body), resp.Header.Get("X-RateLimit-Remaining"))
	switch result.Kind {
	case FailurePrimaryRateLimit:
		result.Kind = FailurePrimaryRateLimit
		result.RetrySource = RetrySourcePrimaryReset
		if result.Snapshot != nil {
			result.RetryAt = result.Snapshot.ResetAt
		}
	case FailureSecondaryRateLimit:
		result.RetryAt, result.RetrySource = retryAfter(resp.Header.Get("Retry-After"), now)
	}
	return result
}

func classifyFailureKind(status int, body, remainingHeader string) FailureKind {
	lower := strings.ToLower(body)
	rateSignal := status == http.StatusTooManyRequests || strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "abuse detection") || strings.Contains(lower, "secondary rate")
	remaining, remainingErr := strconv.Atoi(strings.TrimSpace(remainingHeader))
	if rateSignal && remainingErr == nil && remaining <= 0 {
		return FailurePrimaryRateLimit
	}
	if rateSignal && (status == http.StatusForbidden || status == http.StatusTooManyRequests) {
		return FailureSecondaryRateLimit
	}
	if invalidCredentialFailure(status, lower) {
		return FailureInvalidCredentials
	}
	if status == http.StatusNotFound {
		return FailureMissingResource
	}
	if status >= http.StatusInternalServerError {
		return FailureTransient
	}
	return FailureUnknown
}

func invalidCredentialFailure(status int, lowerBody string) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	return strings.Contains(lowerBody, "bad credentials") ||
		strings.Contains(lowerBody, "resource not accessible") || strings.Contains(lowerBody, "forbidden")
}

func retryAfter(raw string, now time.Time) (time.Time, RetrySource) {
	raw = strings.TrimSpace(raw)
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second).UTC(), RetrySourceRetryAfter
	}
	if parsed, err := http.ParseTime(raw); err == nil {
		return parsed.UTC(), RetrySourceRetryAfter
	}
	return now.Add(secondaryFallbackDelay).UTC(), RetrySourceConservativeFallback
}

func resourceForEndpoint(endpoint string) Resource {
	switch {
	case strings.HasPrefix(endpoint, "/graphql"):
		return ResourceGraphQL
	case strings.HasPrefix(endpoint, "/search/"):
		return ResourceSearch
	default:
		return ResourceCore
	}
}

// FailureKindOf extracts the typed GitHub failure kind through wrapped errors.
func FailureKindOf(err error) FailureKind {
	if err == nil {
		return ""
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		if apiErr.FailureKind != "" {
			return apiErr.FailureKind
		}
		failure := classifyGitHubResponse(
			&http.Response{StatusCode: apiErr.StatusCode, Header: make(http.Header)},
			apiErr.Endpoint,
			[]byte(apiErr.Body),
			time.Now().UTC(),
		)
		return failure.Kind
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return FailureTransient
	}
	return FailureUnknown
}
