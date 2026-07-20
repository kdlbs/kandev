//go:build unix && !linux

package process

import (
	"errors"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
// This allows us to kill all child processes together.
// Note: Pdeathsig is Linux-specific and not available on macOS/BSD.
// On these platforms, orphan cleanup relies on explicit Stop() calls.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func setAgentProcGroup(cmd *exec.Cmd) {
	setProcGroup(cmd)
}

func setManagedProcGroup(cmd *exec.Cmd) {
	setProcGroup(cmd)
}

type processLifecycleHandle struct{}

func installProcessLifecycle(_ *exec.Cmd) (processLifecycleHandle, error) {
	return processLifecycleHandle{}, nil
}

func releaseProcessLifecycle(_ processLifecycleHandle) {}

func reapProcessLifecycle(_ processLifecycleHandle) error { return nil }

func ownsProcessLifecycle(_ processLifecycleHandle) bool { return false }

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

func processGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func isProcessGroupMissing(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
