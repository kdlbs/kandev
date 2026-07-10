//go:build windows

package launcher

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
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
	return winproc.RunTaskkill(args...)
}
