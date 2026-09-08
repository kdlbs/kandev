//go:build linux

package probe

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestProcessGone_ESRCHTreatedSameAsENOENT covers the race where ReadDir
// still lists a pid directory but the kernel finishes tearing down the task
// struct before read() completes: on Linux that surfaces as ESRCH ("no such
// process"), not the ENOENT os.IsNotExist checks for. Both must be treated
// as "process exited, skip it" rather than a fatal snapshot error — see
// readLinuxProcessStat and its caller's "absent, one snapshot" comment.
func TestProcessGone_ESRCHTreatedSameAsENOENT(t *testing.T) {
	esrch := &fs.PathError{Op: "read", Path: "/proc/1/stat", Err: syscall.ESRCH}
	enoent := &fs.PathError{Op: "open", Path: "/proc/1/stat", Err: syscall.ENOENT}
	other := &fs.PathError{Op: "read", Path: "/proc/1/stat", Err: syscall.EACCES}

	if !processGone(esrch) {
		t.Error("ESRCH must be treated as the process having exited")
	}
	if !processGone(enoent) {
		t.Error("ENOENT must be treated as the process having exited")
	}
	if processGone(other) {
		t.Error("an unrelated read error must not be treated as the process having exited")
	}
	if processGone(nil) {
		t.Error("a nil error must not be treated as the process having exited")
	}
}

// TestReadLinuxProcessStat_ESRCHYieldsAbsentNotError exercises the same
// classification through readLinuxProcessStat's actual os.ReadFile call, by
// reading a pid directory that races closed between os.IsNotExist's own
// stat-based check would apply. A pid that never existed reliably returns
// ENOENT from the real filesystem, so this asserts readLinuxProcessStat's
// observable contract for "gone" rather than forcing ESRCH from the kernel.
func TestReadLinuxProcessStat_ESRCHYieldsAbsentNotError(t *testing.T) {
	_, ok, err := readLinuxProcessStat(neverAllocatedPID(t), time.Now())
	if err != nil {
		t.Fatalf("readLinuxProcessStat: got error %v, want nil (absent, not fatal)", err)
	}
	if ok {
		t.Fatal("readLinuxProcessStat: got ok=true for a pid that does not exist")
	}
}

func neverAllocatedPID(t *testing.T) int {
	t.Helper()
	// PID 1 is always init/systemd; a stat file that does not parse as a pid
	// directory at all (e.g. a very high, implausible pid) is guaranteed
	// absent without depending on any process this test spawned.
	const implausiblePID = 1 << 30
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", implausiblePID)); err == nil {
		t.Skip("probe: implausible pid unexpectedly exists on this host")
	}
	return implausiblePID
}
