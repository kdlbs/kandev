// Package parkedprobe holds the KANDEV_PARKED_PROBE_BUDGET parsing logic
// shared by both ends of the parked-on-background-work probe
// (docs/specs/parked-board-mvp/spec.md §7.3): the OS-level process-tree walk
// inside agentctl (internal/agentctl/server/process) and the backend
// orchestrator's WebSocket round trip to it (internal/orchestrator). Kept
// here per apps/backend/CLAUDE.md's cross-tier rule ("put shared logic in a
// neutral internal/common/* package instead of importing across the
// boundary or forking a copy") so the two budgets can never drift apart.
package parkedprobe

import (
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultBudget is the contracted default (AC-81) applied when
// KANDEV_PARKED_PROBE_BUDGET is unset, unparseable, zero, or negative.
const DefaultBudget = 250 * time.Millisecond

// ParseEnvBudget reads KANDEV_PARKED_PROBE_BUDGET and rejects non-positive
// values, logging a warning and returning DefaultBudget in that case
// (AC-81). log may be nil in tests.
func ParseEnvBudget(log *logger.Logger) time.Duration {
	val := os.Getenv("KANDEV_PARKED_PROBE_BUDGET")
	if val == "" {
		return DefaultBudget
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		if log != nil {
			log.Warn("KANDEV_PARKED_PROBE_BUDGET invalid or non-positive, using default",
				zap.String("value", val), zap.Duration("default", DefaultBudget))
		}
		return DefaultBudget
	}
	return d
}
