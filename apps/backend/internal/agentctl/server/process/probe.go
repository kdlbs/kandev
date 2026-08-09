package process

import (
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	probeResultLive    = "live"
	probeResultSettled = "settled"
	probeResultUnknown = "unknown"

	// parkedProbeBudgetDefault is the contracted default (AC-81) applied when
	// KANDEV_PARKED_PROBE_BUDGET is unset, unparseable, zero, or negative.
	parkedProbeBudgetDefault = 250 * time.Millisecond
)

// parseProbeEnvBudget reads KANDEV_PARKED_PROBE_BUDGET and rejects
// non-positive values, logging a warning and returning the 250ms default in
// that case (AC-81). log may be nil in tests.
func parseProbeEnvBudget(log *logger.Logger) time.Duration {
	val := os.Getenv("KANDEV_PARKED_PROBE_BUDGET")
	if val == "" {
		return parkedProbeBudgetDefault
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		if log != nil {
			log.Warn("KANDEV_PARKED_PROBE_BUDGET invalid or non-positive, using default",
				zap.String("value", val), zap.Duration("default", parkedProbeBudgetDefault))
		}
		return parkedProbeBudgetDefault
	}
	return d
}
