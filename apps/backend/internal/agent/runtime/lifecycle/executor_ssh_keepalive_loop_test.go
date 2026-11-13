package lifecycle

import (
	"errors"
	"testing"
	"time"
)

// TestSSHKeepaliveRunLoopPendingStopAndProbeError checks loop-level priority
// with every input queued before startup, without assumptions about scheduling.
//
// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.10
func TestSSHKeepaliveRunLoopPendingStopAndProbeError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stopped bool
	}{{"running", false}, {"stopped", true}} {
		t.Run(tc.name, func(t *testing.T) {
			w := &sshKeepaliveWatchdog{
				deadline: time.Hour,
				stopCh:   make(chan struct{}),
				loopDone: make(chan struct{}),
			}
			errs := make(chan error, 1)
			errs <- errors.New("probe error")
			if tc.stopped {
				w.stopOnce.Do(func() { close(w.stopCh) })
			}
			lost := false
			w.onLost = func(string, time.Duration) { lost = true }
			go w.runLoop(make(chan time.Time), errs, time.Now())
			select {
			case <-w.loopDone:
			case <-time.After(2 * time.Second):
				w.stopAndAwaitLoop()
				t.Fatal("watchdog loop did not finish")
			}
			if lost == tc.stopped {
				t.Fatalf("loss notified = %v, want %v", lost, !tc.stopped)
			}
		})
	}
}
