package backendapp

import (
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"regexp"
	"strconv"
	"time"
)

var rateResetHeader = regexp.MustCompile(`(?i)x-ratelimit-reset["'\\]*\s*[:=]\s*["'\\]*([0-9T:.+Z-]+)`)

// headerResetFromFailure returns the provider's rate-limit reset header only,
// so a classified minute limit never reuses an unrelated long reset hint.
func headerResetFromFailure(failure *routingerr.Error, now time.Time) (time.Time, bool) {
	if failure == nil {
		return time.Time{}, false
	}
	if match := rateResetHeader.FindStringSubmatch(failure.RawExcerpt); len(match) == 2 {
		if reset, ok := parseRateReset(match[1]); ok && reset.After(now) {
			return reset, true
		}
	}
	return time.Time{}, false
}

func parseRateReset(value string) (time.Time, bool) {
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n > 100_000_000_000 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	reset, err := time.Parse(time.RFC3339, value)
	return reset, err == nil
}
