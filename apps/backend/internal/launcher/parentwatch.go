package launcher

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/proclive"
)

const parentWatchPollInterval = 2 * time.Second

type parentWatchdog struct {
	parentPID int64
	interval  time.Duration
	probe     func(int64) (alive, known bool)
	shutdown  func(string)
	exit      func(int)

	mu      sync.Mutex
	started bool
	stopped bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func newParentWatchdog(parentPID int64, shutdown func(string), exit func(int)) *parentWatchdog {
	return &parentWatchdog{
		parentPID: parentPID,
		interval:  parentWatchPollInterval,
		probe:     proclive.Alive,
		shutdown:  shutdown,
		exit:      exit,
	}
}

func newParentWatchdogFromEnv(shutdown func(string), exit func(int)) *parentWatchdog {
	return newParentWatchdog(parentPIDFromEnv(), shutdown, exit)
}

func parentPIDFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv(parentPIDEnv))
	pid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func (w *parentWatchdog) start() {
	w.mu.Lock()
	if w.parentPID <= 0 || w.started || w.stopped {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.stopCh = make(chan struct{})
	w.wg.Add(1)
	w.mu.Unlock()

	go w.run()
}

func (w *parentWatchdog) stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	started := w.started
	if started {
		close(w.stopCh)
	}
	w.mu.Unlock()
	if started {
		w.wg.Wait()
	}
}

func (w *parentWatchdog) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			alive, known := w.probe(w.parentPID)
			if !known || alive {
				continue
			}
			launcherInfof("parent process exited; shutting down supervised processes (parent_pid=%d)", w.parentPID)
			shutdownDebugf("launcher parent watchdog detected exit parent_pid=%d launcher_pid=%d", w.parentPID, os.Getpid())
			w.shutdown("parent process exit")
			w.exit(0)
			return
		}
	}
}
