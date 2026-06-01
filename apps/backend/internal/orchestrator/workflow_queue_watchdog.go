package orchestrator

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// Defaults for the workflow-queue watchdog. The orphan-age threshold gives
// inline paths (#1087/#1096/#1160/#1163) a generous window to handle the
// queue before the watchdog steps in, so the watchdog is purely a safety net
// for the long-tail races those inline paths can't catch.
const (
	defaultWorkflowQueueWatchdogInterval  = 60 * time.Second
	defaultWorkflowQueueWatchdogOrphanAge = 90 * time.Second
	// workflowQueueWatchdogSweepLimit caps the per-tick row scan so a backlog
	// after an outage (thousands of stale entries) can't balloon memory on
	// one sweep. Dedupe by session_id means realistic recovery per tick is
	// bounded by distinct sessions, so this cap rarely bites in practice.
	workflowQueueWatchdogSweepLimit = 500
)

// workflowQueueWatchdog scans the message queue periodically for stale
// workflow auto-start entries whose owning session has no live agent and no
// active turn, and re-fires the auto-resume cascade (tryEnsureExecution →
// ResumeSession → boot_ready → drain). It is a self-healing safety net for
// the long-tail of races where the inline auto-resume hook either short-
// circuits, fails silently, or is bypassed by a non-workflow queue path.
type workflowQueueWatchdog struct {
	svc       *Service
	interval  time.Duration
	orphanAge time.Duration

	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
}

// newWorkflowQueueWatchdog constructs a watchdog tied to the orchestrator
// service. Interval and orphan-age are taken from KANDEV_WORKFLOW_QUEUE_
// WATCHDOG_INTERVAL_SECONDS / _ORPHAN_AGE_SECONDS env vars when set
// (positive integers); otherwise the defaults above apply.
func (s *Service) newWorkflowQueueWatchdog() *workflowQueueWatchdog {
	return &workflowQueueWatchdog{
		svc:       s,
		interval:  durationFromEnvSeconds("KANDEV_WORKFLOW_QUEUE_WATCHDOG_INTERVAL_SECONDS", defaultWorkflowQueueWatchdogInterval),
		orphanAge: durationFromEnvSeconds("KANDEV_WORKFLOW_QUEUE_WATCHDOG_ORPHAN_AGE_SECONDS", defaultWorkflowQueueWatchdogOrphanAge),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start spawns the watchdog loop. NOT idempotent — repeated calls on the
// same instance spawn additional goroutines that all close the same doneCh
// and panic. Call exactly once per watchdog; Stop must follow to drain.
func (w *workflowQueueWatchdog) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the watchdog loop to exit and waits for it to drain. Safe to
// call multiple times (no-op after the first).
func (w *workflowQueueWatchdog) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	<-w.doneCh
}

func (w *workflowQueueWatchdog) run(ctx context.Context) {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

// sweep queries the message queue for stale workflow-tagged entries and
// dispatches at most one recovery action per session per tick.
func (w *workflowQueueWatchdog) sweep(ctx context.Context) {
	if w.svc.messageQueue == nil {
		return
	}
	cutoff := time.Now().Add(-w.orphanAge)
	stale, err := w.svc.messageQueue.ListStaleByQueuedBy(ctx, messagequeue.QueuedByWorkflow, cutoff, workflowQueueWatchdogSweepLimit)
	if err != nil {
		w.svc.logger.Warn("workflow queue watchdog: list stale failed", zap.Error(err))
		return
	}
	if len(stale) == 0 {
		return
	}

	seen := make(map[string]bool, len(stale))
	for i := range stale {
		entry := stale[i]
		if entry.SessionID == "" || seen[entry.SessionID] {
			continue
		}
		seen[entry.SessionID] = true
		w.svc.maybeRecoverOrphanedWorkflowQueue(ctx, entry.SessionID, entry.QueuedAt)
	}
}

// durationFromEnvSeconds reads a positive integer (seconds) from env or falls
// back to def. Any parse error / non-positive value uses def.
func durationFromEnvSeconds(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}
