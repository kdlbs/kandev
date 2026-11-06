//go:build windows

package launcher

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	commonwinproc "github.com/kandev/kandev/internal/common/winproc"
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

func managedProcessGracefulScope(_ bool) gracefulSignalScope {
	return gracefulSignalTreeWide
}

func terminateManagedProcess(pid, _ int, _ bool) error {
	// A Kandev child registers a cooperative shutdown event. Console-less
	// children cannot receive taskkill's window-close request, so use it first.
	if err := commonwinproc.RequestShutdown(pid); err == nil {
		return nil
	}
	// The dev backend is launched through make on Windows. Signalling only make
	// would orphan the backend, so retain tree-wide graceful cleanup there.
	return terminateManagedProcessGroup(pid)
}

func terminateManagedProcessGroup(pid int) error {
	return runLauncherTaskkill("/T", "/PID", fmt.Sprintf("%d", pid))
}

func isManagedProcessTarget(_, _ int) bool {
	return false
}

func managedProcessGroupCleanupSupported() bool {
	return true
}

func runLauncherTaskkill(args ...string) error {
	return winproc.RunTaskkill(args...)
}
