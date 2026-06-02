package instance

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// idleReaperShutdownTimeout bounds how long an idle StopInstance call can
// take before the reaper moves on. Mirrors the timeout used by the regular
// DELETE /instances/{id} HTTP path (control_server.go), so a stuck
// teardown can't block the periodic sweep.
const idleReaperShutdownTimeout = 15 * time.Second

// activityMiddleware wraps a handler so every request bumps the owning
// instance's lastActivity timestamp and inflight counter. The counter is
// kept above zero for the full duration of long-lived requests
// (WebSocket streams), which is the signal the idle reaper uses to treat
// an instance with an open stream as active even when no new HTTP request
// arrives.
func activityMiddleware(inst *Instance) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inst.inflightRequests.Add(1)
			inst.MarkActivity()
			defer func() {
				inst.inflightRequests.Add(-1)
				inst.MarkActivity()
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// runIdleReaper periodically scans instances and stops any that have been
// idle (no in-flight requests, no recent activity) for the configured
// idle timeout. Returns when the manager's stop channel closes.
func (m *Manager) runIdleReaper(timeout, interval time.Duration) {
	defer m.reaperWG.Done()
	if timeout <= 0 {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	m.logger.Info("idle reaper started",
		zap.Duration("idle_timeout", timeout),
		zap.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.reaperStop:
			m.logger.Debug("idle reaper stopped")
			return
		case <-ticker.C:
			m.reapIdleInstances(timeout)
		}
	}
}

// reapIdleInstances identifies and stops every instance that has been idle
// for at least the configured timeout. The scan is snapshotted under the
// instances lock; StopInstance is called outside the lock so a slow
// teardown can't block other Manager operations. The reaper-stop signal
// is polled between each StopInstance so Shutdown doesn't wait for an
// entire idle sweep to drain when callers are trying to exit.
func (m *Manager) reapIdleInstances(timeout time.Duration) {
	now := time.Now()
	idle := m.snapshotIdleInstances(now, timeout)
	for _, id := range idle {
		select {
		case <-m.reaperStop:
			return
		default:
		}
		m.logger.Info("reaping idle agent instance",
			zap.String("instance_id", id),
			zap.Duration("idle_timeout", timeout))
		m.reapSingleIdleInstance(id)
	}
}

// reapSingleIdleInstance wraps StopInstance with a bounded context and
// guarantees cancel() runs even on panic. Kept separate so the timeout
// scope is obvious and `defer cancel()` doesn't pin context lifetimes to
// the whole sweep.
func (m *Manager) reapSingleIdleInstance(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), idleReaperShutdownTimeout)
	defer cancel()
	if err := m.StopInstance(ctx, id); err != nil {
		m.logger.Warn("idle reaper: failed to stop instance",
			zap.String("instance_id", id),
			zap.Error(err))
	}
}

// snapshotIdleInstances returns the IDs of instances whose IsIdle returns true.
// Holding the read lock during the scan keeps StopInstance / CreateInstance
// concurrency intact; iteration happens after the lock is released.
func (m *Manager) snapshotIdleInstances(now time.Time, timeout time.Duration) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0)
	for id, inst := range m.instances {
		if inst != nil && inst.IsIdle(now, timeout) {
			out = append(out, id)
		}
	}
	return out
}
