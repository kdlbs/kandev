// Package scheduler owns the runs queue's claim + dispatch loop. It
// runs a periodic tick (safety net for restart recovery and retry
// scheduling) and consumes an in-process signal channel so rows
// inserted by Service.QueueRun are claimed in milliseconds instead
// of waiting up to one tick (B3.5 in the task-model-unification
// plan).
//
// The scheduler delegates the actual processing — guard checks,
// executor resolution, prompt building, agent launch — to a
// RunProcessor implementation. Today the implementation lives in
// internal/office/service.SchedulerIntegration; that file stays in
// the office package for now because it depends on office-specific
// helpers (skills, env builder, prompt builder). Phase 3.2 will
// migrate the processor into this package alongside the engine
// rewire.
package scheduler

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultTickInterval is the safety-net tick. Engine-emitted rows are
// claimed via the event-driven signal so they don't wait for it; the
// tick exists to recover missed signals (process crash between INSERT
// and signal write), pick up scheduled retries (rows with a future
// scheduled_retry_at), and satisfy "the queue eventually drains" for
// any path that bypasses the signal channel.
const DefaultTickInterval = 5 * time.Second

// SignalCoalesceWindow caps how often consecutive signals trigger a
// claim cycle. When a single QueueRun call lands the channel will
// already buffer one signal; bursts of inserts during a single
// claim cycle don't need to fire a second cycle until this window
// has elapsed.
const SignalCoalesceWindow = 10 * time.Millisecond

// TickIntervalFromEnv reads KANDEV_OFFICE_SCHEDULER_TICK_MS and returns
// the corresponding duration. Falls back to DefaultTickInterval when
// the variable is unset or invalid. The env var name keeps the
// "office" prefix for backward compatibility with existing
// deployments.
func TickIntervalFromEnv() time.Duration {
	raw := os.Getenv("KANDEV_OFFICE_SCHEDULER_TICK_MS")
	if raw == "" {
		return DefaultTickInterval
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return DefaultTickInterval
	}
	return time.Duration(ms) * time.Millisecond
}

// RunProcessor performs one claim-and-dispatch pass. The scheduler
// calls Tick on every wake-up (timer or signal); the processor is
// responsible for draining as many runs as it wants in a single pass
// and returning. Errors are logged and swallowed — the scheduler
// keeps running so a transient failure doesn't stall the queue.
type RunProcessor interface {
	Tick(ctx context.Context)
}

// Scheduler runs the tick + signal loop. It owns no business logic;
// all work lives in the RunProcessor.
type Scheduler struct {
	processor    RunProcessor
	signal       <-chan struct{}
	tickInterval time.Duration
	log          *logger.Logger
}

// New constructs a Scheduler. The signal channel comes from the runs
// service (Service.SubscribeSignal); when nil the scheduler runs as
// a pure tick loop, equivalent to the pre-B3.5 behaviour.
func New(
	processor RunProcessor,
	signal <-chan struct{},
	tickInterval time.Duration,
	log *logger.Logger,
) *Scheduler {
	if tickInterval <= 0 {
		tickInterval = DefaultTickInterval
	}
	return &Scheduler{
		processor:    processor,
		signal:       signal,
		tickInterval: tickInterval,
		log:          log.WithFields(zap.String("component", "runs-scheduler")),
	}
}

// Start runs the tick + signal loop until the context is cancelled.
// Call it from a background goroutine. A single goroutine handles
// both wake-up sources so claim ordering is deterministic and
// per-agent serialisation is unaffected by the new signal path.
func (s *Scheduler) Start(ctx context.Context) {
	s.log.Info("runs scheduler starting",
		zap.Duration("tick_interval", s.tickInterval),
		zap.Bool("signal_enabled", s.signal != nil))

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	// Coalesce burst signals: when a single claim cycle is already
	// running, additional signals don't queue up extra cycles.
	var lastSignal time.Time

	for {
		select {
		case <-ctx.Done():
			s.log.Info("runs scheduler stopping")
			return
		case <-ticker.C:
			s.processor.Tick(ctx)
		case <-s.signal:
			now := time.Now()
			if now.Sub(lastSignal) < SignalCoalesceWindow {
				continue
			}
			lastSignal = now
			s.processor.Tick(ctx)
		}
	}
}
