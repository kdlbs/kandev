package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// rateLimitResetBuffer is added to any parsed reset time to avoid
// exact-second races with the provider's internal counter.
const rateLimitResetBuffer = 30 * time.Second

// rateLimitKeywords are the case-insensitive substrings that identify
// a rate-limit error.
var rateLimitKeywords = []string{
	"rate limit",
	"rate_limit",
	"429",
	"too many requests",
	"quota exceeded",
}

// isRateLimitError returns true when errMsg contains at least one
// rate-limit keyword (case-insensitive).
func isRateLimitError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	for _, kw := range rateLimitKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// Reset-time parsing patterns (tried in order; first match wins).
var (
	// "Retry-After: 3600" or "retry-after 3600"
	retryAfterRe = regexp.MustCompile(`(?i)retry.?after[:\s]+(\d+)`)

	// "try again in 5 minutes" / "try again in 30 seconds" / "try again in 2 hours"
	tryAgainInRe = regexp.MustCompile(`(?i)try again in\s+(\d+)\s*(minute|second|hour)`)

	// "resets at 4:00 AM" / "resets at 04:00" / "reset at 4:00 PM UTC"
	resetsAtRe = regexp.MustCompile(`(?i)resets?\s+at\s+(\d{1,2}):(\d{2})(?:\s*(am|pm))?`)

	// "reset_time: 1234567890" (Unix seconds or milliseconds)
	resetTimestampRe = regexp.MustCompile(`(?i)reset_time[:\s"]+(\d{10,13})`)
)

// parseRateLimitResetTime attempts to extract an absolute UTC retry time
// from errMsg. The buffer (rateLimitResetBuffer) is already included in
// the returned time. Returns nil when no pattern matches.
//
// now is accepted as a parameter so callers (and tests) can fix the clock
// without patching time.Now.
func parseRateLimitResetTime(errMsg string, now time.Time) *time.Time {
	// 1. Retry-After: N seconds
	if m := retryAfterRe.FindStringSubmatch(errMsg); len(m) > 1 {
		if secs, err := strconv.Atoi(m[1]); err == nil {
			t := now.Add(time.Duration(secs)*time.Second + rateLimitResetBuffer)
			return &t
		}
	}

	// 2. "try again in N minutes/seconds/hours"
	if m := tryAgainInRe.FindStringSubmatch(errMsg); len(m) > 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			var d time.Duration
			switch strings.ToLower(m[2]) {
			case "second":
				d = time.Duration(n) * time.Second
			case "minute":
				d = time.Duration(n) * time.Minute
			case "hour":
				d = time.Duration(n) * time.Hour
			}
			if d > 0 {
				t := now.Add(d + rateLimitResetBuffer)
				return &t
			}
		}
	}

	// 3. "resets at HH:MM [AM/PM]" — treated as UTC, advancing to next day if past.
	if m := resetsAtRe.FindStringSubmatch(errMsg); len(m) > 2 {
		if t := parseResetsAt(m[1], m[2], m[3], now); t != nil {
			return t
		}
	}

	// 4. Unix timestamp ("reset_time: 1234567890[000]")
	if m := resetTimestampRe.FindStringSubmatch(errMsg); len(m) > 1 {
		if ts, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			if ts > 1e12 {
				ts /= 1000 // milliseconds → seconds
			}
			t := time.Unix(ts, 0).Add(rateLimitResetBuffer)
			return &t
		}
	}

	return nil
}

// parseResetsAt converts hour/minute/ampm strings into an absolute UTC time.
// If the result is already in the past (relative to now) it advances by 24 h.
func parseResetsAt(hourStr, minStr, ampm string, now time.Time) *time.Time {
	hour, err := strconv.Atoi(hourStr)
	if err != nil {
		return nil
	}
	min, err := strconv.Atoi(minStr)
	if err != nil {
		return nil
	}

	switch strings.ToLower(ampm) {
	case "am":
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour != 12 {
			hour += 12
		}
	}

	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return nil
	}

	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	t = t.Add(rateLimitResetBuffer)
	return &t
}
