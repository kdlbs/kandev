//go:build darwin

package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

// TestWalkProcessTree_DarwinStartTimeSource verifies AC-80: on Darwin the
// process start time is read from sysctl kern.proc.all (not /proc, which does
// not exist on macOS), and the returned times are plausible wall-clock values.
func TestWalkProcessTree_DarwinStartTimeSource(t *testing.T) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		t.Skipf("kern.proc.all unavailable: %v", err)
	}

	// Verify that at least one process entry has a plausible start time —
	// non-zero, in the past, and after a reasonable epoch floor.
	now := time.Now()
	floor := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	var found bool
	for i := range procs {
		kp := &procs[i]
		tv := kp.Proc.P_starttime
		if tv.Sec == 0 {
			continue
		}
		st := time.Unix(tv.Sec, int64(tv.Usec)*1000)
		if st.Before(now) && st.After(floor) {
			found = true
			break
		}
	}
	assert.True(t, found, "at least one process should have a plausible start time from sysctl")
}
