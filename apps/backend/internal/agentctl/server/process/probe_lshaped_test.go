//go:build linux || darwin

package process

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spawnZombieDescendant starts a root shell that execs into a long-lived
// sleep (preserving its PID) after backgrounding a quick-exiting child that
// nothing ever wait()s for — producing a genuine ZOMBIE descendant, not a
// reaped root, for AC-27a's exclusion guard. The exec trick means the
// process occupying the root PID becomes `sleep`, which never reaps children
// forked by its pre-exec shell incarnation, so the backgrounded child
// lingers as a zombie for the test's duration.
func spawnZombieDescendant(t *testing.T) (rootPID int) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sh -c 'true' & exec sleep 300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start root shell")
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	// Give the backgrounded child time to exit and become a zombie.
	time.Sleep(200 * time.Millisecond)
	return cmd.Process.Pid
}

// TestWalkProcessTree_ZombieDescendantExcluded closes the AC-27a test debt
// named in docs/specs/parked-board-mvp/spec.md §9: the existing coverage
// reaped a root, which never exercised walkProcessTree's descendant-zombie
// exclusion. This spawns a real zombie DESCENDANT (see spawnZombieDescendant)
// born after the turn reference, and asserts it is excluded — never reported
// live — matching AC-27a: "an agent whose only in-turn descendant is a
// zombie... reports settled."
func TestWalkProcessTree_ZombieDescendantExcluded(t *testing.T) {
	turnRef := time.Now()
	rootPID := spawnZombieDescendant(t)
	root, ok := captureRootIdentity(rootPID)
	require.True(t, ok, "expected to capture the spawned root's identity")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultSettled, result,
		"a zombie descendant born during the turn must still report settled — zombies are excluded regardless of start time (AC-27a)")
}

// spawnLShapedIdleTree builds the §L-shaped process tree named in AC-70/AC-70a:
// a bridge process (root), one or more CLI processes, and one or more stdio
// MCP server processes — modeled with nested shells rather than the flat
// sh/sleep stand-in every other test in this file uses, so the probe is
// exercised against real multi-level parent/child depth. All three levels
// are born together and stay alive for the test's duration (each level
// `wait`s on a long-lived sleep child).
func spawnLShapedIdleTree(t *testing.T) (bridgePID int) {
	t.Helper()
	// bridge -> CLI (nested sh) -> MCP server (sleep): three real levels,
	// all sharing the bridge's process group.
	cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c "sleep 300 & wait" & wait`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start bridge process")
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	// Give every level time to fork and start running.
	time.Sleep(200 * time.Millisecond)
	return cmd.Process.Pid
}

// TestWalkProcessTree_LShapedIdle closes the AC-70 test debt: the §L-shaped
// idle tree (bridge + CLI + stdio-MCP), all born before the turn start, must
// report settled — the regression guard against sampling process-group
// membership, which would report live here (every level shares the bridge's
// process group; only ppid-chain start-time comparison must decide this).
func TestWalkProcessTree_LShapedIdle(t *testing.T) {
	bridgePID := spawnLShapedIdleTree(t)
	// Turn start captured strictly after every level has materialized, so
	// bridge, CLI, and MCP all predate it.
	turnRef := time.Now()

	root, ok := captureRootIdentity(bridgePID)
	require.True(t, ok, "expected to capture the bridge's root identity")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultSettled, result,
		"an idle §L-shaped tree with every descendant born before turn start must report settled (AC-70)")
}

// TestWalkProcessTree_LShapedLazyMCP closes the AC-70a test debt: the same
// §L-shaped tree, but with one additional stdio MCP server descendant whose
// start time is at or after the current turn's recorded start (the
// lazily-connected case) — the probe must report live. Together with
// TestWalkProcessTree_LShapedIdle this pins that the predicate is the start
// time, not the descendant's identity, command name, or process-group
// membership.
func TestWalkProcessTree_LShapedLazyMCP(t *testing.T) {
	// bridge -> CLI (nested sh), where CLI backgrounds an "old" MCP server
	// immediately and, ~300ms later, a second "lazily-connected" MCP server —
	// both children of CLI, as a stdio MCP server attaching after the
	// bridge/CLI have already been idle for a while.
	cmd := exec.Command("/bin/sh", "-c",
		`/bin/sh -c "sleep 300 & sleep 0.3; sleep 300 & wait" & wait`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start bridge process")
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	// Let bridge, CLI, and the "old" MCP server materialize.
	time.Sleep(150 * time.Millisecond)
	// Turn start falls strictly between the "old" MCP server's start (~t=0)
	// and the "lazily-connected" one spawned ~300ms after CLI started.
	turnRef := time.Now()

	root, ok := captureRootIdentity(cmd.Process.Pid)
	require.True(t, ok, "expected to capture the bridge's root identity")

	// Wait well past the lazily-connected MCP server's spawn point before probing.
	time.Sleep(400 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultLive, result,
		"an §L-shaped tree with one MCP server started at/after turn start must report live (AC-70a) — the predicate is start time, not identity or pgid")
}
