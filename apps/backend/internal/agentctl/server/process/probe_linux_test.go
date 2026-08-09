//go:build linux

package process

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWalkProcessTree_LinuxStartTimeSource verifies AC-80's Linux instance —
// the CI-enforced gate per spec.md's "Start-time source and resolution"
// section (ubuntu-latest is the actual backend job; there is no macOS
// runner). A descendant whose process start time falls in the same 10ms
// tick as the recorded turn start, but strictly after it in nanoseconds,
// must still report "live" — because the turn start is truncated DOWN to
// tick resolution before the inclusive comparison, so the error always
// falls toward "live". This drives the real walkProcessTree path against a
// genuine /proc-backed descendant, not a stub.
//
// turnRef is placed one nanosecond before the end of childStart's own tick
// bucket rather than at a fixed small offset from childStart: linuxBootTime
// reads time.Now() with full nanosecond jitter, so childStart is not itself
// tick-aligned to the truncation boundary (unlike Darwin's kinfo_proc
// start time, which is already microsecond-quantized) — a fixed offset
// could occasionally cross into the next tick and flake.
func TestWalkProcessTree_LinuxStartTimeSource(t *testing.T) {
	parentPID, cleanup := spawnSleepChild(t)
	defer cleanup()

	childStart, ok := linuxChildStartTime(parentPID)
	require.True(t, ok, "expected to find the spawned descendant under /proc")

	bucketStart := childStart.Truncate(linuxProcStatResolution)
	turnRef := bucketStart.Add(linuxProcStatResolution).Add(-time.Nanosecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, parentPID, turnRef)
	assert.Equal(t, probeResultLive, result,
		"a descendant in the same tick as turnRef, but numerically earlier before truncation, must report live")
}

// linuxChildStartTime finds the direct child of parentPID under /proc and
// returns its recorded (tick-resolution) wall-clock start time, using the
// same boot-anchor conversion walkProcessTree does.
func linuxChildStartTime(parentPID int) (time.Time, bool) {
	bootTime, ok := linuxBootTime()
	if !ok {
		return time.Time{}, false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return time.Time{}, false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := linuxReadStat(pid)
		if err != nil || stat.ppid != parentPID {
			continue
		}
		return bootTime.Add(time.Duration(stat.startTicks) * time.Second / linuxClockTicksPerSecond), true
	}
	return time.Time{}, false
}
