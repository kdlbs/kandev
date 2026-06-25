//go:build unix && !linux

package utility

import (
	"errors"
	"os/exec"
	"syscall"
)

func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
