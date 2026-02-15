//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

// terminateProcess sends SIGTERM to the process for graceful shutdown.
func terminateProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// waitPtyProcess waits for the PTY process to exit and returns exit info.
// On Unix, uses cmd.Wait() which inspects WaitStatus for signal information.
func waitPtyProcess(cmd *exec.Cmd, _ PtyHandle) (exitCode int, signalName string, err error) {
	err = cmd.Wait()
	if err == nil {
		return 0, "", nil
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return 1, "", err
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return 1, "", err
	}
	if waitStatus.Signaled() {
		return 128 + int(waitStatus.Signal()), waitStatus.Signal().String(), err
	}
	return waitStatus.ExitStatus(), "", err
}
