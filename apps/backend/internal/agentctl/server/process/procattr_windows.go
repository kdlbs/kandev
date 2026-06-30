//go:build windows

package process

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

// setProcGroup configures the command to run in its own process group.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// setAgentProcGroup configures long-running agent processes for process-tree
// cleanup. On Windows, agents start suspended so installProcessLifecycle can
// bind them to a kill-on-close Job Object before they can spawn descendants.
func setAgentProcGroup(cmd *exec.Cmd) {
	setProcGroup(cmd)
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
}

type processLifecycleHandle struct {
	job winproc.KillOnCloseJob
}

func installProcessLifecycle(cmd *exec.Cmd) (processLifecycleHandle, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return processLifecycleHandle{}, err
	}
	return processLifecycleHandle{job: job}, nil
}

func releaseProcessLifecycle(lifecycle processLifecycleHandle) {
	_ = lifecycle.job.Close()
}

// killProcessGroup kills the entire process tree for the given PID.
// On Windows, we use taskkill with /T flag to kill the process tree.
func killProcessGroup(pid int) error {
	return runProcessTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", pid))
}

// terminateProcessGroup sends a graceful shutdown signal to the process tree.
// Without /F, taskkill sends WM_CLOSE to console and windowed apps, which is
// the closest Windows equivalent of SIGTERM.
func terminateProcessGroup(pid int) error {
	return runProcessTaskkill("/T", "/PID", fmt.Sprintf("%d", pid))
}

func processGroupAlive(_ int) bool {
	// taskkill /T is the authoritative process-tree operation on Windows.
	// There is no Unix-style process group to poll here.
	return false
}

func isProcessGroupMissing(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}

func runProcessTaskkill(args ...string) error {
	return winproc.RunTaskkill(args...)
}
