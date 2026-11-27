package usage

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// @covers AC-COSTS-SUBSCRIPTION-USAGE-001.9
func TestParseRetryAfterBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, value string
		want        time.Duration
	}{
		{"empty", "", 0}, {"whitespace", " \t", 0}, {"seconds", " 60 ", time.Minute},
		{"zero", "0", 0}, {"negative", "-1", 0}, {"negative overflow", "-99999999999999999999", 0},
		{"cap", "900", 15 * time.Minute}, {"over cap", "901", 15 * time.Minute},
		{"int64 maximum", "9223372036854775807", 15 * time.Minute},
		{"positive overflow", "9223372036854775808", 15 * time.Minute},
		{"huge positive", "999999999999999999999999999999999999", 15 * time.Minute},
		{"malformed overflow", "99999999999999999999x", 0}, {"decimal", "1.5", 0},
		{"date", now.Add(2 * time.Minute).Format(http.TimeFormat), 2 * time.Minute},
		{"date cap", now.Add(time.Hour).Format(http.TimeFormat), 15 * time.Minute},
		{"date boundary", now.Format(http.TimeFormat), 0},
		{"past date", now.Add(-time.Second).Format(http.TimeFormat), 0}, {"invalid", "tomorrow", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRetryAfter(tc.value, now); got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestStatusErrorWrappedClassification(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			cause := newStatusError("test", code, http.Header{"Retry-After": {"120"}}, []byte("provider refusal"))
			err := fmt.Errorf("usage caller: %w", cause)
			var status statusCodeHint
			var hint retryAfterHint
			if !errors.Is(err, cause) || !errors.As(err, &status) || status.StatusCode() != code {
				t.Fatalf("lost status/cause: %v", err)
			}
			if !errors.As(err, &hint) || hint.RetryAfter() != 2*time.Minute {
				t.Fatalf("lost retry hint: %v", err)
			}
			if got := isRateLimited(err); got != (code == http.StatusTooManyRequests) {
				t.Fatalf("classification = %v for %v", got, err)
			}
		})
	}
	if isRateLimited(errors.New("429 in an opaque message")) || isRateLimited(nil) {
		t.Fatal("classified an untyped error as rate limited")
	}
}
