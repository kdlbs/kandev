//go:build windows

package process

import (
	"os"
	"os/exec"
)

// terminateProcess kills the process on Windows.
// Windows does not support SIGTERM; process termination is immediate.
func terminateProcess(p *os.Process) error {
	return p.Kill()
}

// waitPtyProcess waits for the PTY process to exit and returns exit info.
// On Windows, uses cmd.Process.Wait() rather than cmd.Wait(): ConPTY started the
// process, so cmd.Wait() would fail with "exec: not started". cmd.Process is an
// independent handle to the same PID (resolved via os.FindProcess in
// ptyexec.Start), so waiting on it does not depend on the ConPTY object's
// lifetime — which is why neither platform needs the PtyHandle here.
func waitPtyProcess(cmd *exec.Cmd) (exitCode int, signalName string, err error) {
	state, err := cmd.Process.Wait()
	if err != nil {
		return 1, "", err
	}
	code := state.ExitCode()
	if code != 0 {
		return code, "", &exec.ExitError{ProcessState: state}
	}
	return 0, "", nil
}
