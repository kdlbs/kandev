// Package ownershipperiod resolves the effective control-server unowned
// period from its two independent inputs. It is cross-tier shared code
// (both the agentctl control server, which enforces the period, and the
// backend, which must renew ownership strictly inside a third of it, need
// the identical computation) and therefore lives in internal/common rather
// than being owned by either side and imported across the boundary.
package ownershipperiod

import "time"

// MinPeriod is the floor of AC-EXECUTORS-CONTROL-OWNERSHIP-003.7: no
// configuration may produce an unowned period at or near zero, which would
// expire ownership before an adopting backend could ever claim it.
const MinPeriod = time.Minute

// Resolve applies AC-EXECUTORS-CONTROL-OWNERSHIP-003.4/.6/.7 to a
// configured unowned period, given the (possibly disabled, i.e. <= 0)
// per-instance idle timeout. It returns the effective period and a
// human-readable adjustment reason for each rule that fired, so the caller
// can log what happened (both criteria require recording the adjustment).
//
// Order matters: the idle-timeout clamp (003.4) is applied first, then the
// one-minute floor (003.7) is applied second and takes precedence -- when
// the idle timeout is short enough that no value satisfies both, the floor
// wins and the ordering constraint is left unsatisfied on purpose (003.7).
func Resolve(configured, idleTimeout time.Duration) (period time.Duration, adjustments []string) {
	period = configured

	// 003.6: a disabled idle timeout (<=0) leaves nothing for the ordering
	// constraint to order, so the clamp of 003.4 is skipped entirely rather
	// than derived from the disabled sentinel.
	if idleTimeout > 0 && period >= idleTimeout {
		period = idleTimeout / 2
		adjustments = append(adjustments,
			"unowned period was not shorter than the idle timeout; clamped to half the idle timeout")
	}

	if period < MinPeriod {
		period = MinPeriod
		adjustments = append(adjustments,
			"unowned period was below the one-minute floor; raised to the floor")
	}

	return period, adjustments
}
