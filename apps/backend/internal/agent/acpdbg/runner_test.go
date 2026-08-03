//go:build !windows

package acpdbg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	pid := waitForPIDFile(t, pidPath)
	t.Cleanup(func() {
		cancel()
		runner.tree.kill(runner.cmd)
		runner.Close("test cleanup")
		if processAlive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

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

func TestRunnerCloseUnblocksFullOOBQueue(t *testing.T) {
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
		for len(runner.oob) > 0 {
			<-runner.oob
		}
		runner.Close("test cleanup")
	})

	waitForOOBQueue(t, runner)
	cancel()
	closed := make(chan struct{})
	go func() {
		runner.Close("context canceled")
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Runner.Close remained blocked on a full OOB queue")
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

func waitForOOBQueue(t *testing.T, runner *Runner) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for len(runner.oob) < cap(runner.oob) {
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for OOB queue to fill: %d/%d", len(runner.oob), cap(runner.oob))
		case <-ticker.C:
		}
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
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
