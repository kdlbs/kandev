//go:build linux

package process

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
// This allows us to kill all child processes together.
// On Linux, we also set Pdeathsig to ensure the child is killed if the parent dies
// unexpectedly (SIGKILL, crash, etc.) without calling Stop().
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

// killProcessGroup kills the entire process group for the given PID.
// Returns nil if successful, or an error if the kill failed.
func killProcessGroup(pid int) error {
	// Kill the entire process group by using negative PID
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// terminateProcessGroup sends SIGTERM to the entire process group for graceful shutdown.
func terminateProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}
