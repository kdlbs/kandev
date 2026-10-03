package github

import (
	"context"
	"encoding/json"
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

const rateLimitedGraphQLErrorBody = "GitHub GraphQL rate limit exceeded"

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
	graphQLRateError := mergeGraphQLRatePayload(endpoint, body, now, &result)
	result.Kind = classifyFailureKind(resp.StatusCode, string(body), resp.Header.Get("X-RateLimit-Remaining"), resp.Header.Get("Retry-After"))
	if graphQLRateError {
		result.Kind = graphQLRateFailureKind(result.Snapshot)
	}
	setGitHubFailureRetry(&result, resp, now)
	return result
}

func mergeGraphQLRatePayload(endpoint string, body []byte, now time.Time, result *githubFailure) bool {
	if !strings.HasPrefix(endpoint, "/graphql") {
		return false
	}
	rateError, snapshot := parseGraphQLRatePayload(body, now)
	if snapshot == nil {
		return rateError
	}
	if result.Snapshot == nil {
		result.Snapshot = snapshot
		return rateError
	}
	mergeGraphQLRateSnapshot(result.Snapshot, snapshot)
	return rateError
}

func graphQLRateFailureKind(snapshot *RateSnapshot) FailureKind {
	if snapshot != nil && snapshot.RemainingObserved && snapshot.Remaining <= 0 {
		return FailurePrimaryRateLimit
	}
	return FailureSecondaryRateLimit
}

func setGitHubFailureRetry(result *githubFailure, resp *http.Response, now time.Time) {
	switch result.Kind {
	case FailurePrimaryRateLimit:
		if result.Snapshot != nil && !result.Snapshot.ResetAt.IsZero() {
			result.RetrySource = RetrySourcePrimaryReset
			result.RetryAt = result.Snapshot.ResetAt
		} else {
			result.RetrySource = RetrySourceConservativeFallback
			result.RetryAt = now.Add(secondaryFallbackDelay).UTC()
		}
		if retryAt, source := retryAfter(resp.Header.Get("Retry-After"), now); source == RetrySourceRetryAfter && retryAt.After(result.RetryAt) {
			result.RetryAt = retryAt
			result.RetrySource = source
		}
	case FailureSecondaryRateLimit:
		result.RetryAt, result.RetrySource = retryAfter(resp.Header.Get("Retry-After"), now)
	}
}

func parseGraphQLRatePayload(body []byte, now time.Time) (bool, *RateSnapshot) {
	var payload struct {
		Errors []graphQLError `json:"errors"`
		Data   struct {
			RateLimit json.RawMessage `json:"rateLimit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, nil
	}
	return hasGraphQLRateError(payload.Errors), parseGraphQLRateSnapshot(payload.Data.RateLimit, now)
}

func hasGraphQLRateError(items []graphQLError) bool {
	for _, item := range items {
		if isGraphQLRateError(item) {
			return true
		}
	}
	return false
}

func isGraphQLRateError(item graphQLError) bool {
	kind := strings.ToLower(item.Type)
	message := strings.ToLower(item.Message)
	return strings.Contains(kind, "rate_limited") || strings.Contains(message, "rate limit") ||
		strings.Contains(message, "secondary rate") || strings.Contains(message, "abuse detection")
}

func parseGraphQLRateSnapshot(raw json.RawMessage, now time.Time) *RateSnapshot {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var rate struct {
		Limit     *int            `json:"limit"`
		Remaining *int            `json:"remaining"`
		ResetAt   json.RawMessage `json:"resetAt"`
	}
	if err := json.Unmarshal(raw, &rate); err != nil {
		return nil
	}
	snapshot := &RateSnapshot{Resource: ResourceGraphQL, UpdatedAt: now}
	if rate.Limit != nil {
		snapshot.Limit = *rate.Limit
	}
	if rate.Remaining != nil {
		snapshot.Remaining = *rate.Remaining
		snapshot.RemainingObserved = true
	}
	if rate.Limit == nil && rate.Remaining == nil && len(rate.ResetAt) == 0 {
		return nil
	}
	snapshot.ResetAt = parseGraphQLResetAt(rate.ResetAt)
	return snapshot
}

func parseGraphQLResetAt(raw json.RawMessage) time.Time {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}
	}
	var reset string
	if err := json.Unmarshal(raw, &reset); err != nil {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, reset)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func mergeGraphQLRateSnapshot(target, payload *RateSnapshot) {
	if payload.Limit != 0 {
		target.Limit = payload.Limit
	}
	if payload.RemainingObserved {
		target.Remaining = payload.Remaining
		target.RemainingObserved = true
	}
	if !payload.ResetAt.IsZero() {
		target.ResetAt = payload.ResetAt
	}
	target.UpdatedAt = payload.UpdatedAt
}

func classifyFailureKind(status int, body, remainingHeader, retryAfterHeader string) FailureKind {
	lower := strings.ToLower(body)
	if primaryRateLimitResponse(status, remainingHeader) {
		return FailurePrimaryRateLimit
	}
	if secondaryRateLimitResponse(status, lower, retryAfterHeader) {
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

// secondaryRateLimitResponse reports whether a response points at secondary
// throttling. Any 429 qualifies after primary exhaustion is ruled out by the
// caller. An explicit rate/abuse signal in the body qualifies at any status
// (GraphQL returns HTTP 200 with a rate-limited errors array), and a
// Retry-After header qualifies on a 403 even when the body is a bare
// "Forbidden" that would otherwise read as an access failure.
func secondaryRateLimitResponse(status int, lowerBody, retryAfterHeader string) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if strings.Contains(lowerBody, "rate_limited") {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	if strings.Contains(lowerBody, "rate limit") || strings.Contains(lowerBody, "abuse detection") ||
		strings.Contains(lowerBody, "secondary rate") {
		return true
	}
	return strings.TrimSpace(retryAfterHeader) != ""
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

	var waited *AdmissionWaitError
	if errors.As(err, &waited) {
		kind, ok := operationRateLimitKind("", waited.Reason)
		if !ok {
			return nil, false
		}
		return newOperationRateLimitDetails(
			kind, waited.Resource, waited.RetryAt, waited.RetrySource, now,
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
