//go:build linux || darwin

package process

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spawnSleepChild starts a parent shell process that in turn spawns a child
// sleep process. Returns the parent PID and a cleanup function.
func spawnSleepChild(t *testing.T) (parentPID int, cleanup func()) {
	t.Helper()
	// The shell spawns "sleep 300" as a background child, then waits for it.
	// This makes the shell process the root and sleep the descendant.
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & wait")
	require.NoError(t, cmd.Start(), "start parent shell")
	// Give the child a moment to appear in the process table.
	time.Sleep(80 * time.Millisecond)
	return cmd.Process.Pid, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// TestWalkProcessTree_Live verifies AC-70: a descendant born after the turn
// reference time causes the probe to return "live".
func TestWalkProcessTree_Live(t *testing.T) {
	turnRef := time.Now().Add(-500 * time.Millisecond) // turn started slightly before shell spawned
	parentPID, cleanup := spawnSleepChild(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, parentPID, turnRef)
	assert.Equal(t, probeResultLive, result, "descendant born after turn ref should be live")
}

// TestWalkProcessTree_PreExisting verifies AC-70a: a descendant born before
// the turn reference time is not counted and the probe returns "settled".
func TestWalkProcessTree_PreExisting(t *testing.T) {
	parentPID, cleanup := spawnSleepChild(t)
	defer cleanup()

	// Turn reference time is 10 seconds in the future, so the sleep child
	// (born now) is older than turnRef - 2s and does not qualify as "live".
	futureRef := time.Now().Add(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, parentPID, futureRef)
	assert.Equal(t, probeResultSettled, result, "pre-existing descendant should not count")
}

// TestWalkProcessTree_Settled verifies AC-71: if the descendant exits before
// the probe runs, the probe returns "settled".
func TestWalkProcessTree_Settled(t *testing.T) {
	// Spawn a shell that exits immediately (no long-lived children).
	cmd := exec.Command("/bin/sh", "-c", "true")
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	// The shell has exited, but its pid may linger briefly in the process table
	// on some kernels. Use a turnRef in the past so any zombie entry wouldn't
	// qualify anyway.
	turnRef := time.Now().Add(-10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Either the pid is already gone (unknown) or settled. Never "live".
	result := walkProcessTree(ctx, cmd.Process.Pid, turnRef)
	assert.NotEqual(t, probeResultLive, result, "exited process should not be live")
}

// TestWalkProcessTree_Unknown verifies AC-72: a non-existent or zero PID
// returns "unknown".
func TestWalkProcessTree_Unknown(t *testing.T) {
	ctx := context.Background()
	turnRef := time.Now()

	t.Run("zero_pid", func(t *testing.T) {
		result := walkProcessTree(ctx, 0, turnRef)
		assert.Equal(t, probeResultUnknown, result)
	})

	t.Run("negative_pid", func(t *testing.T) {
		result := walkProcessTree(ctx, -1, turnRef)
		assert.Equal(t, probeResultUnknown, result)
	})

	t.Run("nonexistent_pid", func(t *testing.T) {
		// PID 999999999 is virtually guaranteed not to exist.
		result := walkProcessTree(ctx, 999999999, turnRef)
		assert.Equal(t, probeResultUnknown, result)
	})
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
