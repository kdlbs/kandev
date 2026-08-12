//go:build darwin

package process

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

// TestWalkProcessTree_DarwinStartTimeSource verifies AC-80's Darwin
// instance: a descendant whose process start time falls in the same
// microsecond tick as the recorded turn start, but strictly after it in
// nanoseconds, must still report "live" — because the turn start is
// truncated DOWN to the source's microsecond resolution before the
// inclusive comparison. A naive, untruncated comparison would read this
// descendant as pre-existing and answer "settled", the expensive direction
// D5 exists to forbid — this is the regression guard for exactly that bug,
// and it drives the real walkProcessTree path (not merely sysctl
// availability), so an implementation reading Darwin start times from
// `ps -eo lstart` (spec.md's forbidden source) fails it too.
//
// This file stays under //go:build darwin because it uses Darwin-only
// x/sys/unix APIs (SysctlKinfoProcSlice, kinfo_proc field layout) that do
// not exist on other GOOS builds — it cannot compile unconditionally. The
// spec's "never a silent pass" requirement is satisfied by
// probe_notdarwin_test.go, a //go:build !darwin sibling that runs the SAME
// test name and explicitly t.Skips, so the test name always appears in a
// CI log — either passed here on a maintainer's Darwin machine, or skipped
// there on ubuntu-latest/windows-latest — never silently absent.
func TestWalkProcessTree_DarwinStartTimeSource(t *testing.T) {
	parentPID := spawnSleepChild(t)
	root, ok := captureRootIdentity(parentPID)
	if !ok {
		t.Skip("kern.proc.pid unavailable for the spawned parent")
	}

	childStart, err := darwinChildStartTime(parentPID)
	if err != nil {
		t.Skipf("kern.proc.all unavailable: %v", err)
	}

	// turnRef is strictly AFTER the descendant's recorded start in raw
	// nanoseconds, but still inside the same microsecond tick — the
	// recorded start carries no sub-microsecond component (the kernel only
	// records microsecond resolution), so adding under 1us keeps it in the
	// same bucket.
	turnRef := childStart.Add(700 * time.Nanosecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := walkProcessTree(ctx, root, newTurnStartMarker(turnRef))
	assert.Equal(t, probeResultLive, result,
		"a descendant in the same microsecond tick as turnRef, but numerically earlier before truncation, must report live")
}

var errDarwinChildNotFound = errors.New("child process not found in kern.proc.all")

// darwinChildStartTime finds the direct child of parentPID in kern.proc.all
// and returns its recorded (microsecond-resolution) start time, reading the
// same source walkProcessTree does.
func darwinChildStartTime(parentPID int) (time.Time, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return time.Time{}, err
	}
	for i := range procs {
		kp := &procs[i]
		if int(kp.Eproc.Ppid) != parentPID {
			continue
		}
		tv := kp.Proc.P_starttime
		if tv.Sec == 0 {
			continue
		}
		return time.Unix(tv.Sec, int64(tv.Usec)*1000), nil
	}
	return time.Time{}, errDarwinChildNotFound
}
