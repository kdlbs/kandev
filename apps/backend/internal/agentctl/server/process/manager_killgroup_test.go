//go:build !windows

package process

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitForProcessExit_KillsProcessGroupOnTimeout is the regression guard
// for GH issue #1247: when an agent subprocess does not exit on stdin close
// (opencode acp), the timeout fallback in waitForProcessExit used to call
// only cmd.Process.Kill(), leaving MCP children re-parented to init.
// After the fix, killProcessGroup is delivered to the leader's pgid so the
// whole tree dies.
func TestWaitForProcessExit_KillsProcessGroupOnTimeout(t *testing.T) {
	log := newTestLogger(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	m := &Manager{
		logger: log,
	}
	m.cmd = fixtureCmd("sleep-with-child " + pidFile + " 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	t.Cleanup(func() {
		// Best-effort: nuke the leader's pgid in case the test left the
		// fixture alive (e.g. assertion failed before reaping).
		_ = killProcessGroup(m.cmd.Process.Pid)
		_, _ = m.cmd.Process.Wait()
	})

	// Wait for the child to be spawned and the pidfile to land. The fixture
	// writes the PID before sleeping; bound the wait so a broken fixture
	// can't hang the test.
	childPID := waitForChildPID(t, pidFile, 5*time.Second)

	// Sanity: the parent is in its own process group, and the child should
	// have inherited it (no setpgid call inside the fixture).
	parentPGID, err := syscall.Getpgid(m.cmd.Process.Pid)
	require.NoError(t, err)
	childPGID, err := syscall.Getpgid(childPID)
	require.NoError(t, err)
	require.Equal(t, parentPGID, childPGID,
		"child must inherit parent's pgid for the group-kill assumption to hold")

	// Keep waitForProcessExit's internal wg.Wait() blocked so the select
	// hits the ctx.Done branch (the path we're regression-testing). The
	// real Start() path Add()s for readStderr/waitForExit; this test
	// bypasses Start so we Add(1) ourselves and Done at the very end so
	// goleak in TestMain stays clean.
	m.wg.Add(1)
	defer m.wg.Done()

	// Drive the timeout fallback path: context is already done, so
	// waitForProcessExit jumps straight to the kill branch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.waitForProcessExit(ctx)

	// Both parent and child must be reaped within a short window. We waited
	// the parent ourselves; for the child we poll /proc (Linux) or use
	// signal-0 probe (portable).
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be killed by process-group reap", childPID)
}

// waitForChildPID polls pidFile until it contains a valid PID or timeout
// expires. Returns the parsed PID. Fails the test on timeout.
func waitForChildPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fixture to write child pid to %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// processAlive reports whether a PID exists and can receive a signal.
// On Unix, sending signal 0 returns nil for live processes and ESRCH for dead.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// signal 0 doesn't deliver anything; it just checks whether the kernel
	// could have delivered a signal — i.e. the process exists.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
