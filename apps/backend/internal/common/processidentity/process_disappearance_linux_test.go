//go:build linux

package processidentity

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func TestProcStatDisappearanceAfterOpen(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	identity, err := Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Open(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "stat"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stat.Close() }()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	_, err = io.ReadAll(stat)
	if !errors.Is(err, syscall.ESRCH) || !processNoLongerExists(err) {
		t.Fatalf("opened stat after process exit: %v, want recognized ESRCH", err)
	}
	terminated, err := OwnedSessionTerminated(identity)
	if err != nil || !terminated {
		t.Fatalf("reaped isolated owner termination: %v, %v", terminated, err)
	}
}

func TestProcStatReadErrorsRemainUnproven(t *testing.T) {
	for _, err := range []error{syscall.EACCES, syscall.EPERM, syscall.EIO} {
		wrapped := &os.PathError{Op: "read", Path: "/proc/123/stat", Err: err}
		if processNoLongerExists(wrapped) {
			t.Fatalf("unproven process read %v was classified as disappearance", wrapped)
		}
	}
	if processNoLongerExists(nil) {
		t.Fatal("successful process read was classified as disappearance")
	}
}
