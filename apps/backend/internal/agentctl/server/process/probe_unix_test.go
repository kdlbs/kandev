//go:build linux || darwin

package process

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

// spawnSleepChild starts a parent shell process that in turn spawns a
// backgrounded child sleep process, and returns the parent PID. The shell
// and its background child run in their own process group (Setpgid) so
// t.Cleanup can kill the whole group: killing only the shell (as an earlier
// version of this helper did) leaves the backgrounded "sleep 300" orphaned
// and running for up to five minutes per test.
func spawnSleepChild(t *testing.T) (parentPID int) {
	t.Helper()
	// The shell spawns "sleep 300" as a background child, then waits for it.
	// This makes the shell process the root and sleep the descendant.
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start parent shell")
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	// Give the child a moment to appear in the process table.
	time.Sleep(80 * time.Millisecond)
	return cmd.Process.Pid
}

// spawnDifferentPgidSleepChild starts a parent bash process that enables job
// control (`set -m`) and backgrounds a sleep as a genuine job — bash puts a
// job-controlled background job into its OWN new process group, distinct
// from the shell's own pgid, exactly as §L measured a Claude-backgrounded
// shell behaves. This is the counterexample spawnSleepChild (used by every
// other walkProcessTree test in this file) cannot produce: plain
// `/bin/sh -c "cmd &"` in a non-interactive script does not enable job
// control, so its "backgrounded" child stays in the SAME process group as
// its parent — the opposite of production, and not a case a regression to
// pgid-based matching would ever fail. Returns the parent's PID
// (walkProcessTree's root) and the backgrounded sleep's PID (a distinct
// process-group leader, reachable only via the ppid chain).
func spawnDifferentPgidSleepChild(t *testing.T) (parentPID, childPID int) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available; set -m job control requires it")
	}

	cmd := exec.Command("bash", "-c", "set -m; sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start parent bash")
	parentPID = cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-parentPID, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pid, ok := findDirectChildPID(parentPID); ok {
			childPID = pid
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotZero(t, childPID, "expected the backgrounded sleep to appear as bash's direct child")
	t.Cleanup(func() {
		// A separate process group from the parent's — not reached by the
		// -parentPID kill above.
		_ = syscall.Kill(-childPID, syscall.SIGKILL)
	})

	// Confirm the two are actually in DIFFERENT process groups — the
	// load-bearing precondition this helper exists to set up. If bash's
	// job-control behavior ever changes, fail loudly here rather than
	// silently testing nothing.
	parentPgid, err := syscall.Getpgid(parentPID)
	require.NoError(t, err)
	childPgid, err := syscall.Getpgid(childPID)
	require.NoError(t, err)
	require.NotEqual(t, parentPgid, childPgid,
		"test setup invariant: the backgrounded sleep must be in a DIFFERENT process group than its parent shell")

	return parentPID, childPID
}

// findDirectChildPID returns the PID of a direct child of parentPID, using
// `ps` for a portable Linux+Darwin parent-child lookup. This is test
// scaffolding to locate a PID for setup/cleanup, not a liveness read — it is
// unrelated to (and does not reintroduce) the spec's prohibition on `ps -eo
// lstart` as a PRODUCTION probe start-time source.
func findDirectChildPID(parentPID int) (int, bool) {
	out, err := exec.Command("ps", "-o", "pid=,ppid=", "-ax").Output()
	if err != nil {
		return 0, false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		if ppid == parentPID {
			return pid, true
		}
	}
	return 0, false
}

// TestWalkProcessTree_DifferentPgidDescendant is AC-71's regression guard for
// the failure mode §L measured and named as the reason process-group
// membership was rejected as the liveness predicate: a backgrounded shell
// job is never a member of its launching shell's process group (job control
// gives it its own), yet IS reachable as a descendant via the ppid chain.
// Every other walkProcessTree test in this file uses spawnSleepChild, whose
// plain `/bin/sh -c "cmd &"` does NOT enable job control, so its
// "backgrounded" child stays in the SAME process group as its parent — the
// opposite of what a production session actually looks like, and not a case
// a regression to pgid-based matching would fail. This test forces the real
// §L shape via spawnDifferentPgidSleepChild.
func TestWalkProcessTree_DifferentPgidDescendant(t *testing.T) {
	parentPID, _ := spawnDifferentPgidSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	require.True(t, ok, "expected to capture the spawned parent's root identity")

	turnRef := time.Now().Add(-500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultLive, result,
		"a descendant in a DIFFERENT process group than its parent, reached only via the ppid chain, must still report live — pgid membership is not the liveness predicate (§L)")
}

// TestWalkProcessTree_Live exercises the general live predicate underlying
// AC-72's second clause: a descendant born after the turn reference time
// causes the probe to return "live". (This comment previously misattributed
// the test to AC-70, which is the dedicated §L-shaped-tree regression guard —
// see TestWalkProcessTree_LShapedIdle / TestWalkProcessTree_LShapedLazyMCP in
// probe_lshaped_test.go for AC-70/AC-70a themselves; §9 test-debt fix.)
func TestWalkProcessTree_Live(t *testing.T) {
	turnRef := time.Now().Add(-500 * time.Millisecond) // turn started slightly before shell spawned
	parentPID := spawnSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	require.True(t, ok, "expected to capture the spawned parent's root identity")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultLive, result, "descendant born after turn ref should be live")
}

// TestWalkProcessTree_PreExisting verifies AC-27b: a descendant born before
// the turn reference time is not counted and the probe returns "settled".
// (This comment previously misattributed the test to AC-70a; §9 test-debt fix.)
func TestWalkProcessTree_PreExisting(t *testing.T) {
	parentPID := spawnSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	require.True(t, ok, "expected to capture the spawned parent's root identity")

	// Turn reference time is 10 seconds in the future, so the sleep child
	// (born now) is older than turnRef - 2s and does not qualify as "live".
	futureRef := time.Now().Add(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(futureRef))
	assert.Equal(t, probeResultSettled, result, "pre-existing descendant should not count")
}

// TestWalkProcessTree_Settled exercises the general no-live-descendant
// predicate: if the only descendant exits before the probe runs, the probe
// never reports "live". (This comment previously misattributed the test to
// AC-71, which is the different-process-group regression guard covered by
// TestWalkProcessTree_DifferentPgidDescendant above; §9 test-debt fix.)
func TestWalkProcessTree_Settled(t *testing.T) {
	// Spawn a shell that exits immediately (no long-lived children).
	cmd := exec.Command("/bin/sh", "-c", "true")
	require.NoError(t, cmd.Start())
	// Capture identity while the process is still at least a zombie entry
	// (before Wait() reaps it) — mirrors production, where identity is
	// captured once at launch and the walk can happen well after exit.
	root, ok := captureRootIdentity(cmd.Process.Pid)
	require.True(t, ok, "expected to capture root identity before the process is reaped")
	require.NoError(t, cmd.Wait())

	// The shell has exited, but its pid may linger briefly in the process table
	// on some kernels. Use a turnRef in the past so any zombie entry wouldn't
	// qualify anyway.
	turnRef := time.Now().Add(-10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Either the pid is already gone (unknown) or settled. Never "live".
	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.NotEqual(t, probeResultLive, result, "exited process should not be live")
}

// TestWalkProcessTree_Unknown verifies INV-1's fail-closed root-identity
// guard: a non-existent, zero, or negative PID returns "unknown". (This
// comment previously misattributed the test to AC-72, which is the
// settled-then-live sequential turn-boundary AC covered in substance by
// TestWalkProcessTree_PreExisting (settled half, AC-27b) and
// TestWalkProcessTree_Live (live half) above; §9 test-debt fix.)
func TestWalkProcessTree_Unknown(t *testing.T) {
	ctx := context.Background()
	turnRef := time.Now()

	marker := newTurnStartMarker(turnRef)

	t.Run("zero_pid", func(t *testing.T) {
		result := walkProcessTree(ctx, rootIdentity{pid: 0}, marker)
		assert.Equal(t, probeResultUnknown, result)
	})

	t.Run("negative_pid", func(t *testing.T) {
		result := walkProcessTree(ctx, rootIdentity{pid: -1}, marker)
		assert.Equal(t, probeResultUnknown, result)
	})

	t.Run("nonexistent_pid", func(t *testing.T) {
		// PID 999999999 is virtually guaranteed not to exist.
		result := walkProcessTree(ctx, rootIdentity{pid: 999999999}, marker)
		assert.Equal(t, probeResultUnknown, result)
	})
}

// TestWalkProcessTree_RejectsReusedPID verifies D5: a root process is
// identified by (pid, start time), never bare pid. A real, currently alive
// process whose recorded identity does not match its actual current start
// time — simulating what a PID-reuse race would look like from
// walkProcessTree's point of view — must report "unknown", never walk that
// process's descendants as if it were the originally recorded one.
func TestWalkProcessTree_RejectsReusedPID(t *testing.T) {
	parentPID := spawnSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	require.True(t, ok, "expected to capture the spawned parent's root identity")

	// Corrupt the captured identity so it no longer matches the real,
	// still-alive process at that PID — exactly what a stale identity
	// captured for a since-exited-and-reused PID would look like.
	stale := root
	stale.startTicks = root.startTicks + 1_000_000
	stale.startTime = root.startTime.Add(-24 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, stale, newTurnStartMarker(time.Now().Add(-500*time.Millisecond)))
	assert.Equal(t, probeResultUnknown, result,
		"a start-time mismatch on an otherwise-alive pid must report unknown, not walk its descendants")
}

// TestParseProbeEnvBudget verifies that non-positive values are rejected
// (AC-81), reading the contracted KANDEV_PARKED_PROBE_BUDGET key with a
// 250ms default.
func TestParseProbeEnvBudget(t *testing.T) {
	const envKey = "KANDEV_PARKED_PROBE_BUDGET"
	const defaultBudget = 250 * time.Millisecond

	cases := []struct {
		name string
		val  string
		want time.Duration
	}{
		{"absent", "", defaultBudget},
		{"valid_10s", "10s", 10 * time.Second},
		{"zero_rejected", "0s", defaultBudget},
		{"negative_rejected", "-1s", defaultBudget},
		{"invalid_rejected", "notaduration", defaultBudget},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.val)
			got := parseProbeEnvBudget(nil)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestManager_AgentPID verifies that AgentPID validates the ACP session ID.
func TestManager_AgentPID(t *testing.T) {
	m := &Manager{}

	pid, ok := m.AgentPID("any-session")
	assert.False(t, ok, "no adapter: should return ok=false")
	assert.Zero(t, pid)
}

// TestManager_RecordAndLastTurnStart verifies the round-trip through atomic storage.
func TestManager_RecordAndLastTurnStart(t *testing.T) {
	m := &Manager{}

	// Before any record: zero time.
	assert.True(t, m.LastTurnStart().IsZero())

	ref := time.Now().Truncate(time.Millisecond)
	m.RecordTurnStart(ref)

	got := m.LastTurnStart()
	// Within 1ms due to truncation to millisecond precision above.
	assert.WithinDuration(t, ref, got, time.Millisecond)
}

// fakeSessionAdapter embeds the interface so the fake satisfies the whole
// AgentAdapter surface while implementing only GetSessionID, the one method
// AgentPID calls.
type fakeSessionAdapter struct {
	adapter.AgentAdapter
	sessionID string
}

func (f *fakeSessionAdapter) GetSessionID() string { return f.sessionID }

// TestManager_ProbeProcessTree_BudgetElapsedResolvesToUnknown verifies AC-46
// condition 2: when KANDEV_PARKED_PROBE_BUDGET is set so tight that
// ProbeProcessTree's own context.WithTimeout deadline has already elapsed by
// the time walkProcessTree runs, the result is "unknown" — never "live" or
// "settled" — and the call returns promptly rather than blocking, matching
// AC-40's "not delayed beyond the budget".
func TestManager_ProbeProcessTree_BudgetElapsedResolvesToUnknown(t *testing.T) {
	t.Setenv("KANDEV_PARKED_PROBE_BUDGET", "1ns")

	const acpSessionID = "acp-session-budget"
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	root, ok := captureRootIdentity(cmd.Process.Pid)
	require.True(t, ok, "expected to capture root identity for the running process")

	m := &Manager{cmd: cmd, adapter: &fakeSessionAdapter{sessionID: acpSessionID}}
	m.agentRootIdentity.Store(root)
	m.RecordTurnStart(time.Now())

	start := time.Now()
	result := m.ProbeProcessTree(context.Background(), acpSessionID)
	elapsed := time.Since(start)

	assert.Equal(t, probeResultUnknown, result, "an already-elapsed budget must resolve to unknown")
	assert.Less(t, elapsed, 2*time.Second, "the probe must not block beyond a reasonable margin over its 1ns budget")
}
