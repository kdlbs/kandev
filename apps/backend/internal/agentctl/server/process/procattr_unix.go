//go:build unix

package process

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
// This allows us to kill all child processes together.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group for the given PID.
// Returns nil if successful, or an error if the kill failed.
func killProcessGroup(pid int) error {
	// Kill the entire process group by using negative PID
	return syscall.Kill(-pid, syscall.SIGKILL)
}

