package process

import (
	"os"
	"time"
)

const (
	probeResultLive    = "live"
	probeResultSettled = "settled"
	probeResultUnknown = "unknown"

	// probeStartTimeSlack is subtracted from the turn reference time when
	// comparing descendant start times. A process forked at turn-start may
	// have its kernel timestamp a few ticks before the user-space reference.
	probeStartTimeSlack = 2 * time.Second
)

// parseProbeEnvBudget reads KANDEV_BACKGROUND_PROBE_BUDGET and rejects
// non-positive values (AC-81), returning the default 5s otherwise.
func parseProbeEnvBudget() time.Duration {
	const defaultBudget = 5 * time.Second
	val := os.Getenv("KANDEV_BACKGROUND_PROBE_BUDGET")
	if val == "" {
		return defaultBudget
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		return defaultBudget
	}
	return d
}
