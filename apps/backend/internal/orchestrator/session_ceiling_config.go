package orchestrator

import (
	"strconv"
	"strings"
)

// maxConcurrentSessionsEnvVar is the only source of an operator-supplied session
// ceiling. There is no YAML setting and no stored setting for it by contract, which
// is why it is registered in the startup configuration inventory as an exclusion
// rather than an entry.
const maxConcurrentSessionsEnvVar = "KANDEV_MAX_CONCURRENT_SESSIONS"

// unlimitedSessionCeiling is the configured value that disables refusal, matching
// the wip_limit convention.
const unlimitedSessionCeiling = 0

// minimumDefaultSessionCeiling floors the derived default so a single-core host
// still admits enough sessions to make progress.
const minimumDefaultSessionCeiling = 2

// defaultSessionCeiling derives the ceiling used when no operator value is set:
// half the host's cores, floored at minimumDefaultSessionCeiling.
func defaultSessionCeiling(numCPU int) int {
	if half := numCPU / 2; half > minimumDefaultSessionCeiling {
		return half
	}
	return minimumDefaultSessionCeiling
}

// resolveSessionCeiling turns the raw environment value into the effective ceiling.
// It returns the rejected raw value when one was supplied but could not be used, so
// the caller can warn; an unset or blank value is not a rejection and takes the
// default silently.
func resolveSessionCeiling(raw string, numCPU int) (ceiling int, rejected string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultSessionCeiling(numCPU), ""
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 0 {
		return defaultSessionCeiling(numCPU), raw
	}
	return parsed, ""
}
