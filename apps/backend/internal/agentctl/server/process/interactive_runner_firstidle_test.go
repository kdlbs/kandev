package process

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// newRunnerWithProcess installs a bare interactiveProcess into the runner's
// map so tests can drive firstIdle semantics without spawning a real PTY.
func newRunnerWithProcess(t *testing.T, processID string) (*InteractiveRunner, *interactiveProcess) {
	t.Helper()
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	proc := &interactiveProcess{
		info: InteractiveProcessInfo{
			ID:        processID,
			SessionID: "sess-" + processID,
			Status:    types.ProcessStatusRunning,
			StartedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
		stopSignal:    make(chan struct{}),
		waitDone:      make(chan struct{}),
		firstOutputCh: make(chan struct{}),
		firstIdleCh:   make(chan struct{}),
	}

	runner.mu.Lock()
	runner.processes[processID] = proc
	runner.mu.Unlock()
	return runner, proc
}

func TestWaitForFirstIdle_fires_on_idle_event(t *testing.T) {
	runner, proc := newRunnerWithProcess(t, "proc-fires")

	done := make(chan error, 1)
	go func() {
		done <- runner.WaitForFirstIdle(context.Background(), "proc-fires")
	}()

	// emitTurnComplete is what fires the idle event in production.
	go runner.emitTurnComplete(proc)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForFirstIdle returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("WaitForFirstIdle did not return after idle fired")
	}
}

func TestWaitForFirstIdle_returns_ctx_error(t *testing.T) {
	runner, _ := newRunnerWithProcess(t, "proc-ctx")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runner.WaitForFirstIdle(ctx, "proc-ctx")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForFirstIdle err = %v, want context.Canceled", err)
	}
}

func TestWaitForFirstIdle_unknown_process_errors(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	err := runner.WaitForFirstIdle(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatalf("WaitForFirstIdle returned nil error for unknown process")
	}
}

func TestEmitTurnComplete_closes_firstIdle_only_once(t *testing.T) {
	runner, proc := newRunnerWithProcess(t, "proc-once")

	runner.emitTurnComplete(proc)
	runner.emitTurnComplete(proc) // second call must not panic on double close

	select {
	case <-proc.firstIdleCh:
		// ok
	default:
		t.Fatalf("firstIdleCh was not closed after emitTurnComplete")
	}
}
