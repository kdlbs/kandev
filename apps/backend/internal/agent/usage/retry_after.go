package usage

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter bounds a provider-supplied Retry-After hint so an absurd or
// hostile header cannot park an account in backoff indefinitely.
const maxRetryAfter = maxBackoffTTL

// retryAfterHint is implemented by provider failures that carry a Retry-After
// hint, so the cache can follow the provider's own pacing.
type retryAfterHint interface {
	RetryAfter() time.Duration
}

// statusCodeHint is implemented by provider failures that carry an HTTP status.
type statusCodeHint interface {
	StatusCode() int
}

// statusError is a non-OK provider response. It keeps the provider status and
// the provider's own retry hint readable by the cache, which cannot recover
// either from an opaque message string.
type statusError struct {
	provider   string
	status     int
	body       string
	retryAfter time.Duration
}

func (e *statusError) Error() string {
	if e.retryAfter > 0 {
		if e.body != "" {
			return fmt.Sprintf("%s usage: unexpected status %d (retry after %s): %s", e.provider, e.status, e.retryAfter.Round(time.Second), e.body)
		}
		return fmt.Sprintf("%s usage: unexpected status %d (retry after %s)", e.provider, e.status, e.retryAfter.Round(time.Second))
	}
	if e.body != "" {
		return fmt.Sprintf("%s usage: unexpected status %d: %s", e.provider, e.status, e.body)
	}
	return fmt.Sprintf("%s usage: unexpected status %d", e.provider, e.status)
}

func (e *statusError) RetryAfter() time.Duration { return e.retryAfter }

func (e *statusError) StatusCode() int { return e.status }

// newStatusError classifies a non-OK provider response, reading the retry hint
// from the response headers while they are still in scope.
func newStatusError(provider string, status int, header http.Header, body []byte) *statusError {
	return &statusError{
		provider:   provider,
		status:     status,
		body:       string(body),
		retryAfter: parseRetryAfter(header.Get("Retry-After"), time.Now()),
	}
}

// parseRetryAfter reads a Retry-After header in either delta-seconds or
// HTTP-date form, clamped to maxRetryAfter. A missing, elapsed, or unreadable
// hint yields zero, which leaves the cache on its own pacing.
func parseRetryAfter(value string, now time.Time) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	seconds, err := strconv.ParseInt(trimmed, 10, 64)
	if errors.Is(err, strconv.ErrRange) && strings.Trim(trimmed, "0123456789") == "" {
		return maxRetryAfter
	}
	var delay time.Duration
	if err == nil {
		if seconds <= 0 {
			return 0
		}
		delay = time.Duration(min(seconds, int64(maxRetryAfter/time.Second))) * time.Second
	} else if parsed, parseErr := http.ParseTime(trimmed); parseErr == nil {
		if !parsed.After(now) {
			return 0
		}
		delay = parsed.Sub(now)
	} else {
		return 0
	}
	return min(delay, maxRetryAfter)
}

// isRateLimited reports whether the failure is the provider refusing the read
// because the account is over its limit.
func isRateLimited(err error) bool {
	var hint statusCodeHint
	return errors.As(err, &hint) && hint.StatusCode() == http.StatusTooManyRequests
}
