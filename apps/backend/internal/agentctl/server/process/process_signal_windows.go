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
// On Windows, uses cmd.Process.Wait() since the process may have been started
// via ConPTY rather than cmd.Start().
func waitPtyProcess(cmd *exec.Cmd, _ PtyHandle) (exitCode int, signalName string, err error) {
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
