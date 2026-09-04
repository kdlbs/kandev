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

// RateLimitErrorCode identifies an operation-local GitHub rate failure.
const RateLimitErrorCode = "github_rate_limited"

// OperationRateLimitKind identifies the rate policy that blocked an operation.
type OperationRateLimitKind string

const (
	OperationRateLimitPrimaryExhaustion  OperationRateLimitKind = "primary_exhaustion"
	OperationRateLimitSecondaryThrottle  OperationRateLimitKind = "secondary_throttle"
	OperationRateLimitInteractiveReserve OperationRateLimitKind = "interactive_reserve"
)

// OperationRateLimitDetails contains safe retry context for a failed operation.
type OperationRateLimitDetails struct {
	Kind              OperationRateLimitKind `json:"kind"`
	Resource          Resource               `json:"resource"`
	RetryAt           *time.Time             `json:"retry_at,omitempty"`
	RetryAfterSeconds int64                  `json:"retry_after_seconds"`
	Source            string                 `json:"source,omitempty"`
}

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
		strings.Contains(lower, "abuse detection") || strings.Contains(lower, "secondary rate") ||
		strings.Contains(lower, "rate_limited")
	if primaryRateLimitResponse(status, remainingHeader) {
		return FailurePrimaryRateLimit
	}
	if rateSignal && (status == http.StatusForbidden || status == http.StatusTooManyRequests ||
		strings.Contains(lower, "rate_limited")) {
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

func primaryRateLimitResponse(status int, remainingHeader string) bool {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return false
	}
	remaining, err := strconv.Atoi(strings.TrimSpace(remainingHeader))
	return err == nil && remaining <= 0
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
	if IsConnectivityError(err) {
		return FailureTransient
	}
	return FailureUnknown
}

// OperationRateLimitFromError returns the safe rate-limit fields that belong
// with a failed Kandev-managed GitHub operation. Successful operations do not
// call this function and do not carry quota details.
func OperationRateLimitFromError(err error, now time.Time) (*OperationRateLimitDetails, bool) {
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		failureKind := apiErr.FailureKind
		if failureKind == "" {
			failureKind = FailureKindOf(apiErr)
		}
		kind, ok := operationRateLimitKind(failureKind, "")
		if !ok {
			return nil, false
		}
		resource := apiErr.Resource
		if resource == "" {
			resource = resourceForEndpoint(apiErr.Endpoint)
		}
		return newOperationRateLimitDetails(
			kind, resource, apiErr.RetryAt, apiErr.RetrySource, now,
		), true
	}

	var deferred *AdmissionDeferredError
	if errors.As(err, &deferred) {
		kind, ok := operationRateLimitKind("", deferred.Reason)
		if !ok {
			return nil, false
		}
		retryAt := deferred.RetryAt
		if retryAt.IsZero() && deferred.Delay > 0 {
			retryAt = now.Add(deferred.Delay).UTC()
		}
		return newOperationRateLimitDetails(
			kind, deferred.Resource, retryAt, deferred.RetrySource, now,
		), true
	}
	return nil, false
}

func operationRateLimitKind(kind FailureKind, admissionReason string) (OperationRateLimitKind, bool) {
	switch {
	case kind == FailurePrimaryRateLimit || admissionReason == rateLimitBlockPrimary:
		return OperationRateLimitPrimaryExhaustion, true
	case kind == FailureSecondaryRateLimit || admissionReason == rateLimitBlockSecondary:
		return OperationRateLimitSecondaryThrottle, true
	case admissionReason == rateLimitBlockPrimaryReserve:
		return OperationRateLimitInteractiveReserve, true
	default:
		return "", false
	}
}

func newOperationRateLimitDetails(
	kind OperationRateLimitKind,
	resource Resource,
	retryAt time.Time,
	retrySource RetrySource,
	now time.Time,
) *OperationRateLimitDetails {
	details := &OperationRateLimitDetails{
		Kind: kind, Resource: resource, Source: publicRetrySource(retrySource),
	}
	if retryAt.IsZero() {
		return details
	}
	retryAt = retryAt.UTC()
	details.RetryAt = &retryAt
	if remaining := retryAt.Sub(now); remaining > 0 {
		details.RetryAfterSeconds = int64((remaining + time.Second - 1) / time.Second)
	}
	return details
}

func publicRetrySource(source RetrySource) string {
	switch source {
	case RetrySourceRetryAfter:
		return "retry_after_header"
	case RetrySourcePrimaryReset:
		return "rate_limit_reset"
	case RetrySourceConservativeFallback:
		return "conservative_fallback"
	default:
		return ""
	}
}
