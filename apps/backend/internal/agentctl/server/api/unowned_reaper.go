package api

import (
	"time"

	"go.uber.org/zap"
)

// unownedReaperScanInterval is how often the reaper checks elapsed unowned
// time. It is a polling granularity, not an operator-facing setting -- like
// F71's log-sink bounds, it is a fixed implementation constant, not one of
// the five numeric tunables in the configuration surface.
const unownedReaperScanInterval = 15 * time.Second

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
// ownership -- the backend-side claim loop in
// internal/agent/runtime/lifecycle: starting it unconditionally on every
// agentctl launch would self-terminate a perfectly healthy attached instance
// once the period elapses, since nothing renews ownership without that
// claim loop. Gated by cmd/agentctl/main.go's startUnownedReaperIfEnabled.
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
			if m.decideUnownedShutdown(period) {
				m.requestShutdown()
				m.logger.Warn("no ownership claim renewed within the unowned period, shutting down",
					zap.Duration("unowned_period", period))
				return
			}
		}
	}
}
