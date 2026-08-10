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
// The marker is built directly in the tick domain — bootTicks: childTicks —
// rather than round-tripped through a wall-clock turnRef and
// newTurnStartMarker's own linuxBootTime() call. /proc/uptime is quantized to
// the same ~10ms as one tick, so two independent linuxBootTime() reads (one
// here, one inside newTurnStartMarker) can disagree by a full tick and flip
// this exact boundary — confirmed in CI, not theoretical. Setting
// bootTicks == childTicks reproduces the same claim AC-80 makes (a same-tick
// descendant counts as live under the inclusive >= comparison) without a
// second boot-time read; newTurnStartMarker's own truncation arithmetic has
// its own dedicated, single-read coverage in
// TestNewTurnStartMarker_ComputesBootTicksOnce below.
func TestWalkProcessTree_LinuxStartTimeSource(t *testing.T) {
	parentPID := spawnSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	require.True(t, ok, "expected to capture the spawned parent's root identity")

	childTicks, ok := linuxChildStartTime(parentPID)
	require.True(t, ok, "expected to find the spawned descendant under /proc")

	marker := turnStartMarker{bootTicks: childTicks, hasBootTicks: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, marker)
	assert.Equal(t, probeResultLive, result,
		"a descendant whose ticks equal the recorded turn-start ticks must report live (inclusive >=)")
}

// TestNewTurnStartMarker_ComputesBootTicksOnce is the regression guard for
// Review round 6's clock-domain finding: a prior implementation stored the
// turn start as a raw wall-clock time.Time and reconverted every
// descendant's ticks into wall time using a FRESH boot anchor read at probe
// time — the direction the spec's clock-domain rule (docs/specs/
// disambiguate-waiting/spec.md, "The clock DOMAIN, stated per platform")
// forbids, because a wall-clock adjustment between stamp and probe shifts
// the anchor and can push an in-turn descendant before the (re-derived)
// turn start. This test pins the fix's conversion arithmetic directly:
// newTurnStartMarker must convert the wall-clock stamp into boot-relative
// ticks immediately, using the boot anchor read at that same call — so
// round-tripping a wall time built from a known tick offset off the live
// boot anchor reproduces that exact tick count, truncated DOWN (AC-80's
// "error always falls toward live" rule), with no later re-derivation step
// in the picture at all.
func TestNewTurnStartMarker_ComputesBootTicksOnce(t *testing.T) {
	bootTime, ok := linuxBootTime()
	require.True(t, ok, "expected /proc/uptime to be readable in CI")

	const wantTicks = int64(12345)
	// Land 4ms into the tick so truncation must discard the remainder,
	// proving the conversion truncates DOWN rather than to the nearest tick.
	wallTime := bootTime.Add(time.Duration(wantTicks)*linuxProcStatResolution + 4*time.Millisecond)

	marker := newTurnStartMarker(wallTime)
	require.True(t, marker.hasBootTicks, "expected a live boot anchor to produce hasBootTicks=true")
	assert.Equal(t, wantTicks, marker.bootTicks,
		"boot ticks must be the tick count as of newTurnStartMarker's own call, truncated down, not re-derived later")
	assert.Equal(t, wallTime, marker.wallTime)
}

// linuxChildStartTime finds the direct child of parentPID under /proc and
// returns its raw ticks-since-boot start time. Returning raw ticks rather
// than a wall-clock time.Time lets callers build a turnStartMarker directly
// in the tick domain, with no linuxBootTime() call (and no boundary-flipping
// double-read risk) needed at all.
func linuxChildStartTime(parentPID int) (int64, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
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
		return stat.startTicks, true
	}
	return 0, false
}
