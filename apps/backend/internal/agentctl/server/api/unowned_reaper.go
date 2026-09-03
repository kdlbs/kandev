package api

import (
	"time"

	"go.uber.org/zap"
)

// minUnownedPeriod is the floor of AC-EXECUTORS-CONTROL-OWNERSHIP-003.7: no
// configuration may produce an unowned period at or near zero, which would
// expire ownership before an adopting backend could ever claim it.
const minUnownedPeriod = time.Minute

// unownedReaperScanInterval is how often the reaper checks elapsed unowned
// time. It is a polling granularity, not an operator-facing setting -- like
// F71's log-sink bounds, it is a fixed implementation constant, not one of
// the five numeric tunables in the configuration surface.
const unownedReaperScanInterval = 15 * time.Second

// resolveUnownedPeriod applies AC-EXECUTORS-CONTROL-OWNERSHIP-003.4/.6/.7 to
// a configured unowned period, given the (possibly disabled, i.e. <= 0)
// per-instance idle timeout. It returns the effective period and a
// human-readable adjustment reason for each rule that fired, so the caller
// can log what happened (both criteria require recording the adjustment).
//
// Order matters: the idle-timeout clamp (003.4) is applied first, then the
// one-minute floor (003.7) is applied second and takes precedence -- when
// the idle timeout is short enough that no value satisfies both, the floor
// wins and the ordering constraint is left unsatisfied on purpose (003.7).
func resolveUnownedPeriod(configured, idleTimeout time.Duration) (period time.Duration, adjustments []string) {
	period = configured

	// 003.6: a disabled idle timeout (<=0) leaves nothing for the ordering
	// constraint to order, so the clamp of 003.4 is skipped entirely rather
	// than derived from the disabled sentinel.
	if idleTimeout > 0 && period >= idleTimeout {
		period = idleTimeout / 2
		adjustments = append(adjustments,
			"unowned period was not shorter than the idle timeout; clamped to half the idle timeout")
	}

	if period < minUnownedPeriod {
		period = minUnownedPeriod
		adjustments = append(adjustments,
			"unowned period was below the one-minute floor; raised to the floor")
	}

	return period, adjustments
}

// StartUnownedReaper starts the background goroutine that stops every
// instance and exits once no ownership claim has been current for the
// resolved unowned period (AC-EXECUTORS-CONTROL-OWNERSHIP-003.1). It fires
// the SAME one-way shutdown latch and ShutdownRequested signal the
// ownership-shutdown operation uses (see ownership.go), so the run loop
// only needs one trigger for both a deliberate shutdown call and a timed-out
// one.
//
// Callers must not start this until the launch is actually running with the
// agent-survival capability engaged AND something is periodically renewing
// ownership (the backend-side claim loop, added in a later layer): starting
// it unconditionally on every agentctl launch would self-terminate a
// perfectly healthy attached instance once the period elapses, since
// nothing renews ownership without that claim loop. Not yet called from
// NewControlServer or cmd/agentctl/main.go for exactly that reason.
func (m *ControlServer) StartUnownedReaper() {
	m.reaperWG.Add(1)
	go m.runUnownedReaper(m.unownedPeriod, unownedReaperScanInterval)
}

// StopUnownedReaper signals the reaper goroutine to exit and waits for it
// to drain. Safe to call even if StartUnownedReaper was never called.
func (m *ControlServer) StopUnownedReaper() {
	m.reaperStopOnce.Do(func() { close(m.reaperStop) })
	m.reaperWG.Wait()
}

func (m *ControlServer) runUnownedReaper(period, interval time.Duration) {
	defer m.reaperWG.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.reaperStop:
			return
		case <-ticker.C:
			if m.ownership.UnownedFor() >= period {
				m.logger.Warn("no ownership claim renewed within the unowned period, shutting down",
					zap.Duration("unowned_period", period))
				m.ownership.BeginShutdown()
				m.requestShutdown()
				return
			}
		}
	}
}
