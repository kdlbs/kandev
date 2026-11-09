package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
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

const (
	stopWaitSnapshotCooldown = 15 * time.Minute
	stopWaitSnapshotLimit    = 8 << 20
	maxStopWaitSnapshots     = 4
	stopWaitSnapshotPrefix   = "kandev-stop-wait-"
	stopWaitSnapshotSuffix   = ".goroutines"
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

func (w *stopWaitWatch) advance(phase stopWaitPhase) {
	if w != nil {
		w.phase.Store(phase)
	}
}

func (w *stopWaitWatch) finish() {
	w.once.Do(func() {
		w.state.CompareAndSwap(0, 1)
		close(w.done)
	})
}

var lastStopWaitSnapshot atomic.Int64

// reportStopWaitDiagnostic records a stop warning and bounded goroutine
// snapshot. The cooldown keeps a fan-out stall to one snapshot per process.
func reportStopWaitDiagnostic(log *logger.Logger, executionID string, phase stopWaitPhase) {
	logStopWaitWarning(log, executionID, phase)
	dir, err := defaultStopWaitSnapshotDir()
	if err != nil {
		log.Warn("stop wait snapshot unavailable",
			zap.String("execution_id", executionID),
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
		return
	}
	captureStopWaitSnapshotInDir(log, executionID, phase, dir)
}

func reportStopWaitDiagnosticInDir(log *logger.Logger, executionID string, phase stopWaitPhase, dir string) {
	logStopWaitWarning(log, executionID, phase)
	captureStopWaitSnapshotInDir(log, executionID, phase, dir)
}

func logStopWaitWarning(log *logger.Logger, executionID string, phase stopWaitPhase) {
	log.Warn("agent stop is taking longer than expected",
		zap.String("execution_id", executionID),
		zap.String("phase", string(phase)),
	)
}

func defaultStopWaitSnapshotDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, "kandev", "stop-wait"), nil
}

func captureStopWaitSnapshotInDir(log *logger.Logger, executionID string, phase stopWaitPhase, dir string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Warn("stop wait snapshot unavailable",
			zap.String("execution_id", executionID),
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
		return
	}
	now := time.Now().UnixNano()
	var previous int64
	for {
		last := lastStopWaitSnapshot.Load()
		if last != 0 && now-last < int64(stopWaitSnapshotCooldown) {
			return
		}
		if lastStopWaitSnapshot.CompareAndSwap(last, now) {
			previous = last
			break
		}
	}
	file, err := os.CreateTemp(dir, stopWaitSnapshotPrefix+"*.goroutines")
	if err != nil {
		lastStopWaitSnapshot.CompareAndSwap(now, previous)
		log.Warn("stop wait snapshot unavailable",
			zap.String("execution_id", executionID),
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
		return
	}
	writer := &stopWaitLimitedWriter{writer: file, remaining: stopWaitSnapshotLimit}
	if profile := pprof.Lookup("goroutine"); profile != nil {
		err = profile.WriteTo(writer, 1)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file.Name())
		lastStopWaitSnapshot.CompareAndSwap(now, previous)
		log.Warn("stop wait snapshot failed",
			zap.String("execution_id", executionID),
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
		return
	}
	if err := pruneStopWaitSnapshots(dir, file.Name()); err != nil {
		log.Warn("stop wait snapshot retention cleanup failed",
			zap.String("execution_id", executionID),
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
	}
	log.Warn("stop wait goroutine snapshot",
		zap.String("execution_id", executionID),
		zap.String("phase", string(phase)),
		zap.String("goroutine_snapshot", file.Name()),
		zap.Bool("snapshot_truncated", writer.truncated),
	)
}

func pruneStopWaitSnapshots(dir, currentPath string) error {
	return pruneStopWaitSnapshotsWithRemove(dir, currentPath, os.Remove)
}

func pruneStopWaitSnapshotsWithRemove(dir, currentPath string, remove func(string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read stop wait snapshot directory: %w", err)
	}
	type snapshotFile struct {
		path    string
		name    string
		modTime time.Time
	}
	files := make([]snapshotFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, stopWaitSnapshotPrefix) || !strings.HasSuffix(name, stopWaitSnapshotSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect stop wait snapshot %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, snapshotFile{
			path:    filepath.Join(dir, name),
			name:    name,
			modTime: info.ModTime(),
		})
	}
	currentPath = filepath.Clean(currentPath)
	sort.Slice(files, func(i, j int) bool {
		if files[i].path == currentPath {
			return files[j].path != currentPath
		}
		if files[j].path == currentPath {
			return false
		}
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name > files[j].name
		}
		return files[i].modTime.After(files[j].modTime)
	})
	// Retain at most four stop-wait snapshots in the user cache directory.
	var removeErrors []error
	for _, file := range files[min(maxStopWaitSnapshots, len(files)):] {
		if err := remove(file.path); err != nil && !os.IsNotExist(err) {
			removeErrors = append(removeErrors, fmt.Errorf("remove old stop wait snapshot %q: %w", file.name, err))
		}
	}
	return errors.Join(removeErrors...)
}

type stopWaitLimitedWriter struct {
	writer    io.Writer
	remaining int
	truncated bool
}

func (w *stopWaitLimitedWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		if len(p) > 0 {
			w.truncated = true
		}
		return len(p), nil
	}
	writeSize := min(len(p), w.remaining)
	if writeSize < len(p) {
		w.truncated = true
	}
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
