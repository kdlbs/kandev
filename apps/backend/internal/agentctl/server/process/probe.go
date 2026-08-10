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

// rootIdentity pins a process to (pid, start time) per D5 ("a process is
// identified by (pid, start time), never bare pid"), so walkProcessTree can
// detect a reused PID instead of trusting a bare existence check. The
// concrete comparison value is platform-specific — raw kernel
// ticks-since-boot on Linux (startTicks), an absolute kernel
// timeval-derived time.Time on Darwin (startTime) — captured once by
// captureRootIdentity right after the process starts and compared later
// against a fresh read of whatever process currently holds that PID.
// Comparing each platform's native unit directly, rather than converting
// both reads through a wall-clock boot-time anchor, avoids re-derivation
// jitter between two independent reads of the same underlying value.
// Platforms without a real implementation (captureRootIdentity always
// returns ok=false) ignore this type entirely.
type rootIdentity struct {
	pid        int
	startTicks int64
	startTime  time.Time
}

// turnStartMarker pins the turn-start instant for the descendant-liveness
// comparison, captured once at RecordTurnStart time. Concrete fields are
// platform-specific, mirroring rootIdentity above: wallTime is the raw
// stamp, used directly by platforms that compare in the wall-clock domain
// (Darwin, whose p_starttime is already wall-clock). bootTicks/hasBootTicks
// are Linux-only — computed once, at newTurnStartMarker (stamp) time, from
// the boot anchor read at that same moment, so the comparison at probe time
// never re-reads a boot anchor and re-derives every descendant's apparent
// start. Re-deriving at probe time is the direction the spec's clock-domain
// rule forbids (docs/specs/disambiguate-waiting/spec.md, "The clock DOMAIN,
// stated per platform"): a wall-clock adjustment between stamp and probe
// would shift the anchor and could push an in-turn descendant before the
// (re-derived) turn start, flipping a live workload to falsely "settled".
// Platforms that never populate bootTicks compare in the wall domain and
// ignore it entirely.
type turnStartMarker struct {
	wallTime     time.Time
	bootTicks    int64
	hasBootTicks bool
}

// isZero reports whether no turn has been recorded yet.
func (m turnStartMarker) isZero() bool {
	return m.wallTime.IsZero()
}

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
