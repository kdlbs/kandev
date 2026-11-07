package lifecycle

import (
	"io"
	"os"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type stopWaitPhase string

const (
	stopWaitRemoteRecovery stopWaitPhase = "remote_recovery"
	stopWaitRemoteLock     stopWaitPhase = "remote_lifecycle_lock"
	stopWaitActivity       stopWaitPhase = "activity"
	stopWaitCheckpoint     stopWaitPhase = "preserve_turn_changes"
	stopWaitAgentctl       stopWaitPhase = "agentctl_stop"
	stopWaitFileCleanup    stopWaitPhase = "project_file_cleanup"
	stopWaitRuntime        stopWaitPhase = "runtime_stop"
	stopWaitRuntimeCleanup stopWaitPhase = "runtime_cleanup"
	stopWaitDetachClient   stopWaitPhase = "detach_client"
	stopWaitPersistence    stopWaitPhase = "persist_terminal_state"
	stopWaitRemove         stopWaitPhase = "remove_tracking"
	stopWaitPublish        stopWaitPhase = "stopped_event_publish"
)

type stopWaitWatch struct {
	phase atomic.Value
	done  chan struct{}
	once  sync.Once
	state atomic.Uint32
}

func newStopWaitWatch(after time.Duration, report func(stopWaitPhase)) *stopWaitWatch {
	watch := &stopWaitWatch{done: make(chan struct{})}
	watch.phase.Store(stopWaitRemoteRecovery)
	go func() {
		timer := time.NewTimer(after)
		defer timer.Stop()
		select {
		case <-watch.done:
		case <-timer.C:
			if watch.state.CompareAndSwap(0, 2) {
				report(watch.phase.Load().(stopWaitPhase))
			}
		}
	}()
	return watch
}

func (w *stopWaitWatch) advance(phase stopWaitPhase) { w.phase.Store(phase) }

func (w *stopWaitWatch) finish() {
	w.once.Do(func() {
		w.state.CompareAndSwap(0, 1)
		close(w.done)
	})
}

var lastStopWaitSnapshot atomic.Int64

// captureStopWaitSnapshot records stack sites without goroutine arguments. A
// short process-wide cooldown keeps a fan-out stall to one local artifact.
func captureStopWaitSnapshot(log *logger.Logger, executionID string, phase stopWaitPhase) {
	now := time.Now().UnixNano()
	for {
		last := lastStopWaitSnapshot.Load()
		if now-last < int64(15*time.Minute) {
			return
		}
		if lastStopWaitSnapshot.CompareAndSwap(last, now) {
			break
		}
	}
	file, err := os.CreateTemp("", "kandev-stop-wait-*.goroutines")
	if err != nil {
		log.Warn("stop wait snapshot unavailable", zap.String("execution_id", executionID), zap.String("phase", string(phase)), zap.Error(err))
		return
	}
	writer := &stopWaitLimitedWriter{writer: file, remaining: 8 << 20}
	if profile := pprof.Lookup("goroutine"); profile != nil {
		err = profile.WriteTo(writer, 1)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	log.Warn("stop wait goroutine snapshot",
		zap.String("execution_id", executionID), zap.String("phase", string(phase)),
		zap.String("goroutine_snapshot", file.Name()), zap.Bool("snapshot_truncated", writer.remaining == 0), zap.Error(err))
}

type stopWaitLimitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *stopWaitLimitedWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return len(p), nil
	}
	writeSize := min(len(p), w.remaining)
	n, err := w.writer.Write(p[:writeSize])
	w.remaining -= n
	if err != nil {
		return n, err
	}
	if n < writeSize {
		return n, io.ErrShortWrite
	}
	return len(p), nil
}
