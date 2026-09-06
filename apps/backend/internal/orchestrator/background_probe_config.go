package orchestrator

import (
	"os"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Env vars and defaults for the background-workload liveness probe (spec
// docs/specs/disambiguate-waiting/spec.md, §"Timing, configuration...").
// Both are plain env config, not runtimeflags/registry.go entries — the
// spec is explicit these are operational tuning, not a release gate.
const (
	envParkedProbeBudget   = "KANDEV_PARKED_PROBE_BUDGET"
	envParkedProbeInterval = "KANDEV_PARKED_PROBE_INTERVAL"

	defaultParkedProbeBudget   = 250 * time.Millisecond
	defaultParkedProbeInterval = 30 * time.Second
)

// BackgroundProbeConfig holds the two operational tuning knobs for the
// background-workload liveness probe.
type BackgroundProbeConfig struct {
	// Budget bounds the synchronous turn-settle probe call (D2). Zero or
	// negative is rejected at load time (warn-logged, default used) — a
	// deliberate exception to the "0 disables" idiom: 0 here would enable
	// an unbounded blocking call on the turn-settle path (round-5 F10/F81).
	Budget time.Duration

	// Interval is the periodic re-sample cadence while parked (task-05).
	// Zero disables periodic sampling — that is valid, documented
	// behaviour, unlike Budget. Negative is rejected the same way as
	// Budget (round-5 F10: time.NewTicker panics on a non-positive value).
	Interval time.Duration
}

// LoadBackgroundProbeConfig reads and validates KANDEV_PARKED_PROBE_BUDGET
// and KANDEV_PARKED_PROBE_INTERVAL (AC-81). log may be nil in tests.
func LoadBackgroundProbeConfig(log *logger.Logger) BackgroundProbeConfig {
	return BackgroundProbeConfig{
		Budget:   loadParkedProbeDuration(log, envParkedProbeBudget, defaultParkedProbeBudget, false),
		Interval: loadParkedProbeDuration(log, envParkedProbeInterval, defaultParkedProbeInterval, true),
	}
}

// loadParkedProbeDuration parses key as a duration, rejecting anything
// non-positive (and additionally rejecting exactly zero unless allowZero) by
// warn-logging and falling back to defaultValue. Missing or unparseable
// values also fall back to defaultValue.
func loadParkedProbeDuration(log *logger.Logger, key string, defaultValue time.Duration, allowZero bool) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		if log != nil {
			log.Warn("invalid duration for env var, using default",
				zap.String("env", key), zap.String("value", raw), zap.Duration("default", defaultValue))
		}
		return defaultValue
	}

	if parsed < 0 || (parsed == 0 && !allowZero) {
		if log != nil {
			log.Warn("non-positive value rejected for env var, using default",
				zap.String("env", key), zap.Duration("value", parsed), zap.Duration("default", defaultValue))
		}
		return defaultValue
	}

	return parsed
}
