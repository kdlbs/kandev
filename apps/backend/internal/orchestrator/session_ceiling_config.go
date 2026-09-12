package orchestrator

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"go.uber.org/zap"
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

// newSessionCeilingControllerFromEnv resolves maxConcurrentSessionsEnvVar exactly
// once, at controller construction, and builds the controller from the result.
// A rejected raw value is logged at WARN here rather than propagated, since this
// is the only production call site and the caller has nothing further to do with
// it besides log it.
func newSessionCeilingControllerFromEnv(lister admittedSessionLister, logger *zap.Logger) *sessionCeilingController {
	ceiling, rejected := resolveSessionCeiling(os.Getenv(maxConcurrentSessionsEnvVar), runtime.NumCPU())
	if rejected != "" {
		effectiveLogger := logger
		if effectiveLogger == nil {
			effectiveLogger = zap.NewNop()
		}
		effectiveLogger.Warn("session ceiling environment value could not be parsed; using the derived default",
			zap.String("env_var", maxConcurrentSessionsEnvVar),
			zap.String("value", rejected),
			zap.Int("default", ceiling))
	}
	return newSessionCeilingController(ceiling, lister, logger)
}

// newSessionCeilingForRepo builds the controller the service owns, binding the
// repository as the persisted half of its population source.
//
// The repository is reached through a narrow consumer-side interface and a type
// assertion, following the idiom the other repository consumers in this package
// use. That idiom degrades silently, and here the degraded state is dangerous
// rather than merely reduced: an unbound controller still answers, but it counts
// only its own in-process reservations, so every session already running on the
// instance is invisible to it and the ceiling admits without bound. A build-time
// assertion covers the production repository; this warning covers everything else.
func newSessionCeilingForRepo(repo interface{}, logger *zap.Logger) *sessionCeilingController {
	if logger == nil {
		logger = zap.NewNop()
	}
	lister, ok := repo.(admittedSessionLister)
	if !ok {
		logger.Warn("repository cannot enumerate admitted sessions; the session ceiling will count only in-process reservations",
			zap.String("env_var", maxConcurrentSessionsEnvVar))
		lister = nil
	}
	return newSessionCeilingControllerFromEnv(lister, logger)
}
