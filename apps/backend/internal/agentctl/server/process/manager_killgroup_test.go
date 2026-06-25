//go:build !windows

package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/stretchr/testify/require"
)

// TestWaitForProcessExit_KillsProcessGroupOnTimeout is the regression guard
// for GH issue #1247: when an agent subprocess does not exit on stdin close
// (opencode acp), the timeout fallback in waitForProcessExit used to call
// only cmd.Process.Kill(), leaving MCP children re-parented to init.
// After the fix, killProcessGroup is delivered to the leader's pgid so the
// whole tree dies.
func TestWaitForProcessExit_KillsProcessGroupOnTimeout(t *testing.T) {
	log, observed := newObservedTestLogger(t)
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
	require.True(t, observedLogsContain(observed, "agent process group SIGKILL requested"),
		"shutdown should log the process-group SIGKILL attempt")

	// Both parent and child must be reaped within a short window. We waited
	// the parent ourselves; for the child we treat both "process gone" and
	// "process exists in zombie state" as success — in CI the orphan
	// re-parents to a container's PID 1 (tini-style) that may take longer
	// than the kill itself to harvest the zombie, but for our purposes the
	// SIGKILL has already done its job once the kernel marks the task as
	// dead. 15s window covers slow CI runners on Linux.
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 15*time.Second, 50*time.Millisecond,
		"child process %d should be killed by process-group reap", childPID)
}

func TestWaitForProcessExit_ContextCanceledWaitsAfterForceKill(t *testing.T) {
	log := newTestLogger(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	m := &Manager{
		logger: log,
	}
	m.cmd = fixtureCmd("sleep-with-child " + pidFile + " 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	parentPID := m.cmd.Process.Pid

	waitDone := make(chan struct{})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		_ = m.cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = killProcessGroup(parentPID)
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for fixture parent to exit")
		}
	})

	childPID := waitForChildPID(t, pidFile, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.waitForProcessExit(ctx)

	select {
	case <-waitDone:
	default:
		t.Fatal("waitForProcessExit returned before command wait completed after force kill")
	}
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be killed by process-group force kill", childPID)
}

func TestWaitForProcessExit_ReapsProcessGroupAfterLeaderExit(t *testing.T) {
	log := newTestLogger(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	m := &Manager{
		logger: log,
	}
	m.cmd = fixtureCmd("exit-with-child " + pidFile + " 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	parentPID := m.cmd.Process.Pid

	waitDone := make(chan struct{})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		_ = m.cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = killProcessGroup(parentPID)
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for fixture parent to exit")
		}
	})

	childPID := waitForChildPID(t, pidFile, 5*time.Second)
	childPGID, err := syscall.Getpgid(childPID)
	require.NoError(t, err)
	require.Equal(t, parentPID, childPGID,
		"child must remain in the leader's pgid after the leader exits")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m.waitForProcessExit(ctx)

	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be reaped even after the group leader exits", childPID)
}

func TestWaitForExit_ReapsProcessGroupAfterNaturalLeaderExit(t *testing.T) {
	log := newTestLogger(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	m := &Manager{
		logger:    log,
		updatesCh: make(chan adapter.AgentEvent, 1),
		doneCh:    make(chan struct{}),
	}
	m.status.Store(StatusRunning)
	m.cmd = fixtureCmd("exit-with-child " + pidFile + " 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	parentPID := m.cmd.Process.Pid
	t.Cleanup(func() {
		_ = killProcessGroup(parentPID)
	})

	childPID := waitForChildPID(t, pidFile, 5*time.Second)
	m.wg.Add(1)
	go m.waitForExit()

	select {
	case <-m.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process manager waitForExit")
	}
	require.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be reaped when the leader exits naturally", childPID)
}

func TestWaitForProcessExit_TerminatesBeforeParentContextDeadline(t *testing.T) {
	log, observed := newObservedTestLogger(t)

	m := &Manager{
		logger: log,
	}
	m.cmd = fixtureCmd("sleep 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	parentPID := m.cmd.Process.Pid

	waitDone := make(chan struct{})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		_ = m.cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = killProcessGroup(parentPID)
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for fixture parent to exit")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startedAt := time.Now()
	m.waitForProcessExit(ctx)
	elapsed := time.Since(startedAt)

	require.Less(t, elapsed, 700*time.Millisecond,
		"shutdown should terminate a non-exiting agent without spending a full second in local grace")
	require.True(t, observedLogsContain(observed, "agent process group SIGTERM requested"),
		"shutdown should log the process-group SIGTERM attempt")
	require.Eventually(t, func() bool {
		return !processAlive(parentPID)
	}, 5*time.Second, 50*time.Millisecond,
		"agent process %d should be killed after local graceful wait", parentPID)
}

func TestStop_DoesNotReapStaleProcessGroupWhenStatusAlreadyStopped(t *testing.T) {
	log, observed := newObservedTestLogger(t)

	m := &Manager{
		logger: log,
	}
	m.status.Store(StatusStopped)
	m.cmd = fixtureCmd("sleep 30")
	setProcGroup(m.cmd)
	require.NoError(t, m.cmd.Start())
	parentPID := m.cmd.Process.Pid

	waitDone := make(chan struct{})
	go func() {
		_ = m.cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = killProcessGroup(parentPID)
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for fixture parent to exit")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, m.Stop(ctx))
	require.True(t, processAlive(parentPID), "stopped manager should not signal a cached process PID")
	require.False(t, observedLogsContain(observed, "agent process group SIGTERM requested"),
		"stopped manager should not attempt process-group termination")
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

// processAlive reports whether a PID exists, is not a zombie, and can
// receive a signal. A zombie is treated as dead — the task has already
// been killed; we're just waiting for the parent (or init) to harvest the
// exit status, which is irrelevant to the regression we're testing for.
//
// On Linux we consult /proc/<pid>/stat to detect zombie state (field 3,
// `R` running, `S` sleeping, `D` uninterruptible, `Z` zombie, `T` stopped).
// On other Unix platforms we fall back to signal(0), which still catches
// the leak case in the bug report — opencode acp processes live with
// state 'S' on Linux until killed.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "linux" {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return false // /proc entry gone => process gone
		}
		// Format: `<pid> (<comm>) <state> ...`. comm may contain spaces,
		// so anchor on the trailing ")" before the state field.
		s := string(raw)
		idx := strings.LastIndex(s, ") ")
		if idx == -1 || idx+2 >= len(s) {
			return false
		}
		state := s[idx+2]
		return state != 'Z'
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
