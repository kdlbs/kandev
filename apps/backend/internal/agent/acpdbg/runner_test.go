//go:build !windows

package acpdbg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestRunnerContextCancellationKillsProcessTree(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "process-tree-helper",
		Command: []string{"sh", "-c", fmt.Sprintf("sleep 30 & child=$!; printf '%%s' $child > %q; wait", pidPath)},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	pid := 0
	t.Cleanup(func() {
		cancel()
		runner.tree.kill(runner.cmd)
		runner.Close("test cleanup")
		if processAlive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	pid = waitForPIDFile(t, pidPath)

	cancel()
	closed := make(chan struct{})
	go func() {
		runner.Close("context canceled")
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Runner.Close did not return after context cancellation")
	}

	waitForProcessGone(t, pid)
}

// TestRunnerCloseFailsPendingRequest pins that shutdown returns promptly and
// hands every waiting caller an error, with out-of-band frames still queued.
// It was written when a bounded queue could block the read loop; the queue is
// unbounded now, so what it still guards is the pending-request teardown.
func TestRunnerCloseFailsPendingRequest(t *testing.T) {
	const frame = `{"jsonrpc":"2.0","method":"session/update"}`
	cmd := fmt.Sprintf("for i in $(seq 1 40); do printf '%%s\\n' '%s'; done; sleep 30", frame)
	ctx, cancel := context.WithCancel(context.Background())
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "oob-helper",
		Command: []string{"sh", "-c", cmd},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		runner.tree.kill(runner.cmd)
		runner.Close("test cleanup")
	})

	requestDone := make(chan error, 1)
	go func() {
		_, err := runner.Request(context.Background(), Frame{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "probe",
		})
		requestDone <- err
	}()
	waitForPendingRequest(t, runner)
	waitForOOBQueue(t, runner, 40)
	cancel()
	closed := make(chan struct{})
	go func() {
		runner.Close("context canceled")
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Runner.Close did not return after cancellation with OOB frames queued")
	}
	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("pending Request returned nil after runner shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("pending Request remained blocked after runner shutdown")
	}
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw, err := os.ReadFile(path); err == nil {
			pid, err := strconv.Atoi(string(raw))
			if err != nil {
				t.Fatalf("parse child pid %q: %v", string(raw), err)
			}
			return pid
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for child pid file %s", path)
		case <-ticker.C:
		}
	}
}

// waitForOOBQueue blocks until at least want frames are queued out of band. The
// queue is unbounded, so there is no "full" state to wait for; callers pass the
// number of frames the fixture emits, which also proves none were dropped.
func waitForOOBQueue(t *testing.T, runner *Runner, want int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		runner.oobMu.Lock()
		queued := len(runner.oobBuf)
		runner.oobMu.Unlock()
		if queued >= want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d OOB frames, got %d", want, queued)
		case <-ticker.C:
		}
	}
}

func waitForPendingRequest(t *testing.T, runner *Runner) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		runner.mu.Lock()
		pending := len(runner.pending)
		runner.mu.Unlock()
		if pending > 0 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for pending Request")
		case <-ticker.C:
		}
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// Linux keeps killed children as zombies until their parent reaps them.
	// A zombie is no longer running, so do not treat it as a surviving
	// descendant while the test waits for the process group to settle.
	if runtime.GOOS == "linux" {
		if raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			if end := bytes.LastIndexByte(raw, ')'); end >= 0 && end+2 < len(raw) {
				return raw[end+2] != 'Z'
			}
		}
	}
	return true
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for processAlive(pid) {
		select {
		case <-deadline.C:
			t.Fatalf("process %d survived shutdown", pid)
		case <-ticker.C:
		}
	}
}
