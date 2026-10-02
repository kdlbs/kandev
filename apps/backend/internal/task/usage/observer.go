package usage

import (
	"context"
	"expvar"
	"time"

	"go.uber.org/zap"
)

// observerQueueCapacity bounds the notices waiting for the observer.
const observerQueueCapacity = 256

// observerCallTimeout bounds one observer call.
const observerCallTimeout = 45 * time.Second

// observerDroppedTotal counts notices abandoned because the queue was full.
var observerDroppedTotal = expvar.NewInt("coordinator_usage_observer_dropped_total")

// RecordedObserver is told, after a usage row commits, which task and session
// it belongs to. The context is cancelled when the writer stops.
type RecordedObserver func(ctx context.Context, taskID, sessionID string)

type usageNotice struct{ taskID, sessionID string }

// SetRecordedObserver installs the single observer, replacing any earlier one;
// nil clears it. It has no effect once the writer is stopping. The observer in
// force when a notice is dequeued is the one that runs.
func (w *Writer) SetRecordedObserver(fn RecordedObserver) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.draining {
		return
	}
	w.observer = fn
}

func (w *Writer) currentObserver() RecordedObserver {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.observer
}

// offerNotice queues a notice without ever blocking the worker.
func (w *Writer) offerNotice(taskID, sessionID string) {
	if w.currentObserver() == nil || taskID == "" || sessionID == "" {
		return
	}
	select {
	case w.notices <- usageNotice{taskID: taskID, sessionID: sessionID}:
	default:
		observerDroppedTotal.Add(1)
		if w.log != nil {
			w.log.Warn("usage observer queue full, notice dropped", zap.String("task_id", taskID))
		}
	}
}

// runObserver is the single consumer: notices run serially in arrival order
// and are discarded once the writer stops.
func (w *Writer) runObserver() {
	defer w.obsWG.Done()
	for n := range w.notices {
		if w.obsCtx.Err() != nil {
			continue
		}
		if fn := w.currentObserver(); fn != nil {
			w.callObserver(fn, n)
		}
	}
}

func (w *Writer) callObserver(fn RecordedObserver, n usageNotice) {
	ctx, cancel := context.WithTimeout(w.obsCtx, observerCallTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil && w.log != nil {
			w.log.Warn("usage observer panicked", zap.Any("panic", r), zap.String("task_id", n.taskID))
		}
	}()
	fn(ctx, n.taskID, n.sessionID)
}
