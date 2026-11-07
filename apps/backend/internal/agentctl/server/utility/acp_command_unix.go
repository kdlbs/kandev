//go:build unix && !linux

package utility

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type acpCommandLifecycleHandle struct{}

func installACPCommandLifecycle(_ *exec.Cmd) (acpCommandLifecycleHandle, error) {
	return acpCommandLifecycleHandle{}, nil
}

func releaseACPCommandLifecycle(_ acpCommandLifecycleHandle) {}

func confirmInferenceTermination(_ context.Context, _ acpCommandLifecycleHandle, pid int) bool {
	return recoveredInferenceTerminated(pid)
}

func recoveredInferenceTerminated(pid int) bool {
	return pid > 0 && errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH)
}

func terminateACPProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func killACPProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func acpProcessGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func acpProcessGroupMissing(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
