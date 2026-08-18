//go:build linux || darwin

package launcher

import (
	"os/exec"
	"os/signal"
	"syscall"
)

func ignoreBrokenPipeSignal() {
	signal.Ignore(syscall.SIGPIPE)
}

func configureManagedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killManagedProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func managedProcessGracefulScope(targetReady bool) gracefulSignalScope {
	if targetReady {
		return gracefulSignalRootOnly
	}
	return gracefulSignalTreeWide
}

func terminateManagedProcess(rootPID, targetPID int, targetReady bool) error {
	if targetReady {
		return syscall.Kill(targetPID, syscall.SIGTERM)
	}
	return syscall.Kill(-rootPID, syscall.SIGTERM)
}

func terminateManagedProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func isManagedProcessTarget(rootPID, targetPID int) bool {
	pgid, err := syscall.Getpgid(targetPID)
	return err == nil && pgid == rootPID
}

func managedProcessGroupCleanupSupported() bool {
	return true
}
