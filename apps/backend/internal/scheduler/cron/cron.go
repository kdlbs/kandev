// Package cron owns the Phase 5 cron-driven trigger handlers (see
// docs/specs/task-model-unification/plan.md, B5.1–B5.4).
//
// Three handlers run on a shared tick loop:
//
//   - HeartbeatHandler iterates (task, step) pairs whose steps have an
//     on_heartbeat trigger, gates them on cooldown / paused / archived,
//     and fires engine.TriggerOnHeartbeat per eligible pair so the step's
//     queue_run action emits a run.
//   - BudgetHandler scans configured budget policies, detects threshold
//     crossings, and fires engine.TriggerOnBudgetAlert so the step's
//     on_budget_alert action decides what to queue (typically one run on
//     the workspace coordination task rather than fanning out per-task).
//   - RoutinesHandler is a thin adapter over the existing
//     office/routines RoutineService.TickScheduledTriggers — routine cron
//     behaviour is unchanged in Phase 5; the only collapse is that the
//     newly-created task's first step on_enter handles the wakeup via
//     the engine instead of needing a manual task_assigned signal.
//
// All three handlers share a single Loop so the backend has one cron
// goroutine driving all phase-5 timers. Handlers are independent — each
// one's Tick errors are logged and swallowed so a transient failure in
// one handler does not stall the others.
package cron

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultTickInterval is the shared cron tick. 30s is short enough that
// a heartbeat configured for "every 60s" fires close to schedule (next
// tick after the cooldown elapses) and long enough that the cron
// goroutine doesn't churn CPU when the system is idle. Individual
// handlers apply their own cadence on top of this tick — a handler
// that hasn't been due since its last fire returns immediately.
const DefaultTickInterval = 30 * time.Second

// Handler is one logical cron consumer wired into the shared loop.
// Implementations MUST be safe for concurrent calls (the loop fires
// each handler in its own goroutine per tick) but typically they are
// invoked serially per handler.
type Handler interface {
	// Name returns a stable identifier used in logs and telemetry. It
	// SHOULD be a short snake_case string (e.g. "heartbeat", "budget").
	Name() string
	// Tick performs one pass. Errors returned here are logged by the
	// loop and otherwise swallowed — they MUST NOT panic.
	Tick(ctx context.Context) error
}

// Loop drives a shared ticker that invokes every registered handler
// once per tick.
type Loop struct {
	interval time.Duration
	handlers []Handler
	log      *logger.Logger
}

// NewLoop constructs a Loop. A non-positive interval falls back to
// DefaultTickInterval. The handler order is preserved across ticks but
// each handler runs in its own goroutine per tick so a slow handler
// does not block its peers.
func NewLoop(interval time.Duration, log *logger.Logger, handlers ...Handler) *Loop {
	if interval <= 0 {
		interval = DefaultTickInterval
	}
	return &Loop{
		interval: interval,
		handlers: handlers,
		log:      log.WithFields(zap.String("component", "scheduler-cron")),
	}
}

// Start runs the loop until ctx is cancelled. Call from a background
// goroutine. The first tick fires after interval — handlers must not
// rely on a "tick at t=0" semantic.
func (l *Loop) Start(ctx context.Context) {
	if len(l.handlers) == 0 {
		l.log.Info("cron loop start: no handlers, exiting")
		return
	}
	names := make([]string, 0, len(l.handlers))
	for _, h := range l.handlers {
		names = append(names, h.Name())
	}
	l.log.Info("cron loop starting",
		zap.Duration("interval", l.interval),
		zap.Strings("handlers", names))

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.log.Info("cron loop stopping")
			return
		case <-ticker.C:
			l.fanOut(ctx)
		}
	}
}

// fanOut invokes every handler concurrently and waits for them all to
// return. Concurrent fan-out keeps a slow handler (e.g. a budget query
// hitting a transient SQLite lock) from delaying the others by up to a
// full tick.
func (l *Loop) fanOut(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(len(l.handlers))
	for _, h := range l.handlers {
		go func(h Handler) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					l.log.Error("cron handler panic",
						zap.String("handler", h.Name()),
						zap.Any("recover", r))
				}
			}()
			if err := h.Tick(ctx); err != nil {
				l.log.Error("cron handler tick failed",
					zap.String("handler", h.Name()),
					zap.Error(err))
			}
		}(h)
	}
	wg.Wait()
}
