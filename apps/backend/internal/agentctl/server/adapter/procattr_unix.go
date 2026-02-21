//go:build unix && !linux

package adapter

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group for the given PID.
func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
