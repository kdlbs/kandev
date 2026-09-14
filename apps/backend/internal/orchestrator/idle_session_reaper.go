package orchestrator

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Idle-session reaper.
//
// The reaper is a single owner of one background goroutine on Service. It
// periodically scans executors_running rows for sessions that have been
// idle (no live agent process, no active turn, session state in
// WaitingForInput/Idle/Completed) past a minimum-idle threshold, then
// hands each off to the same fail-closed reclaimIdleSession primitive
// that the synchronous settle points (subtask terminal collapse,
// agent.completed post-settle) use. Rows whose owner process is
// still alive are never touched: IsAgentRunningForSession returns
// true and reclaimIdleSession aborts.
//
// Two constants:
//   - idleReaperInterval: how often the reaper scans. Short enough to
//     reclaim idle rows within a minute, long enough to avoid per-tick
//     amplification when no rows are candidates.
//   - idleReaperMinIdle: the minimum row age (UpdatedAt → now) before a
//     row is eligible. New rows are never reaped even if their state
//     already looks idle — this gives the system a settling window
//     after a normal turn ends so a row that is "WaitingForInput
//     since 1ms" does not get reclaimed on the very next tick.
//
// Both constants are intentional configuration knobs in code source —
// not env vars — because the orchestrator has no other runtime config
// of this shape, and the values are bounded by fail-closed invariants
// rather than deployment shape. Operators who need to tune them can
// edit and re-deploy; consumers do not.
//
// Hard invariants:
//   - Never call StopAgent / StopAgentWithReason / Kill. reclaimIdleSession
//     routes through repairDeadRowLiveness (status=stopped, LocalPID=0,
//     resume_token preserved); a live executor is short-circuited inside
//     reclaimIdleSession before any side effect.
//   - reclaimIdleSession is fail-closed: an uncertain guard is a skip,
//     never a force. The reaper just calls the primitive in a loop,
//     so the same invariant carries.
//   - Rows whose UpdatedAt is unset / zero / in the future are treated
//     as ineligible; they would otherwise be reaped on the first tick.
//   - The reaper scans regardless of session lifecycle: it does NOT
//     depend on session.ensure / session.launch / agent.ready. The
//     startup reconciliation covers fresh restarts; the reaper covers
//     ongoing drift.
const (
	// idleReaperInterval is the cadence at which the reaper scans
	// executors_running for reclaim candidates. Short enough to keep
	// idle windows tight, long enough that no tick overlaps with the
	// previous tick on a busy system.
	idleReaperInterval = 30 * time.Second

	// idleReaperMinIdle is the minimum age (now - row.UpdatedAt)
	// before a row is eligible. Below this, the system is still
	// settling — on_enter / on_turn_complete / agent.ready may have
	// just transitioned the row and the next event has not fired yet.
	idleReaperMinIdle = 60 * time.Second
)

// idleSessionReaper owns the lifecycle of one background goroutine.
// All methods are nil-safe: a Service with reaper == nil treats the
// reaper as off (useful for tests that don't want a loop).
type idleSessionReaper struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	started  bool
	interval time.Duration
	minIdle  time.Duration
}

func newIdleSessionReaper() *idleSessionReaper {
	return &idleSessionReaper{
		interval: idleReaperInterval,
		minIdle:  idleReaperMinIdle,
	}
}

// start launches the reaper loop with the given tick callback.
// Idempotent: a second call on a running reaper is a no-op. Tests
// inject a custom tick to observe invocation count; production
// wires Service.reclaimIdleSessionsOnce as the tick.
func (r *idleSessionReaper) start(parent context.Context, tick func(ctx context.Context)) bool {
	if r == nil {
		return false
	}
	if tick == nil {
		return false
	}
	if parent == nil {
		parent = context.Background()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return false
	}
	loopCtx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.started = true
	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.runLoop(loopCtx, tick)
	}()
	return true
}

// stop signals the reaper to exit and waits for it. Safe to call on
// a never-started / already-stopped reaper. Blocks until the loop
// observes cancellation; tests and Service.Stop rely on this for
// clean teardown.
func (r *idleSessionReaper) stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	r.workers.Wait()
	r.mu.Lock()
	r.cancel = nil
	r.started = false
	r.mu.Unlock()
}

// runLoop selects on the ticker + the loop context. Tick callback runs
// under the loop context, so a slow tick is cancelled on shutdown.
func (r *idleSessionReaper) runLoop(ctx context.Context, tick func(ctx context.Context)) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick(ctx)
		}
	}
}

// Service-level hooks. The reaper is owned by Service so the lifecycle
// matches the rest of the orchestration: Service.Start launches it,
// Service.Stop joins it.

// startIdleSessionReaper wires the reaper into Service.Start. The
// reaper captures the canonical fail-closed reclaim primitive so
// the per-tick work is a thin scan + loop, not a new policy. The tick also
// drives reclaimStuckSignalSessionsOnce (stuck_signal_watchdog.go),
// detectOfficeDecisionWaitingOnce (office_decision_stall_watchdog.go) and
// reapStalePendingMovesOnce (pending_move_reaper.go) — further scans
// sharing this ticker rather than background goroutines of their own,
// preserving the reaper's single-goroutine-owner invariant. The 30s
// cadence is far finer than either of those needs; they are here for the
// goroutine budget, not for the interval.
func (s *Service) startIdleSessionReaper(ctx context.Context) {
	if s.idleReaper == nil {
		return
	}
	if !s.idleReaper.start(ctx, func(tickCtx context.Context) {
		s.reclaimIdleSessionsOnce(tickCtx)
		s.reclaimStuckSignalSessionsOnce(tickCtx)
		s.detectOfficeDecisionWaitingOnce(tickCtx)
		s.reapStalePendingMovesOnce(tickCtx)
	}) {
		return
	}
	s.logger.Info("idle-session reaper started",
		zap.Duration("interval", s.idleReaper.interval),
		zap.Duration("min_idle", s.idleReaper.minIdle))
}

// stopIdleSessionReaper joins the reaper. Must be called before
// Service.Stop returns to honour the goroutine-ownership invariant.
func (s *Service) stopIdleSessionReaper() {
	if s.idleReaper == nil {
		return
	}
	s.idleReaper.stop()
	s.logger.Info("idle-session reaper stopped")
}

// reclaimIdleSessionsOnce is the per-tick scan. It reads every
// executors_running row, applies the minimum-idle filter (UpdatedAt
// age), and delegates each candidate to reclaimIdleSession. The
// primitive enforces the fail-closed guard (state, live agent,
// active turn); this scan only adds the age filter that the
// settle-point callers do not need.
//
// Each row's reclaim is best-effort: a failure is logged at warn and
// skipped. The next tick will retry. The scan never aborts the loop
// on a single-row error.
func (s *Service) reclaimIdleSessionsOnce(ctx context.Context) {
	now := time.Now().UTC()
	cutoff := now.Add(-s.idleReaper.minIdle)
	var (
		rows []*models.ExecutorRunning
		err  error
	)
	if candidates, ok := s.repo.(idleExecutorCandidateLister); ok {
		rows, err = candidates.ListExecutorsRunningIdle(ctx, cutoff)
	} else {
		rows, err = s.repo.ListExecutorsRunning(ctx)
	}
	if err != nil {
		s.logger.Warn("idle reaper: list executors_running failed; tick skipped",
			zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Status == models.ExecutorRunningStatusStopped {
			continue
		}
		sessionID := row.SessionID
		if sessionID == "" {
			continue
		}
		// Reject rows with zero / unset / future UpdatedAt — they would
		// otherwise look older than the threshold on the first tick.
		if row.UpdatedAt.IsZero() {
			continue
		}
		if row.UpdatedAt.After(now) || now.Sub(row.UpdatedAt) < s.idleReaper.minIdle {
			continue
		}
		// Delegate to the fail-closed primitive. It reads the session,
		// checks the triple guard (state × live runtime × active turn),
		// and either reclaims the row or skips it. Errors here are
		// recoverable on the next tick — log and move on.
		if err := s.reclaimIdleSession(ctx, sessionID); err != nil {
			s.logger.Warn("idle reaper: reclaim failed; row preserved for next tick",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// idleExecutorCandidateLister is implemented by the durable repository. The
// fallback to ListExecutorsRunning keeps lightweight test doubles compatible,
// while production can filter stopped and young rows in SQL.
type idleExecutorCandidateLister interface {
	ListExecutorsRunningIdle(ctx context.Context, cutoff time.Time) ([]*models.ExecutorRunning, error)
}
