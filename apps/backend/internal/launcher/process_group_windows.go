//go:build windows

package launcher

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func ignoreBrokenPipeSignal() {
}

func configureManagedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killManagedProcessGroup(pid int) error {
	return runLauncherTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", pid))
}

func terminateManagedProcessGroup(pid int) error {
	return runLauncherTaskkill("/T", "/PID", fmt.Sprintf("%d", pid))
}

func runLauncherTaskkill(args ...string) error {
	output, err := exec.Command("taskkill", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}
