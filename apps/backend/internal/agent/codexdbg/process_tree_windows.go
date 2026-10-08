//go:build windows

package codexdbg

import (
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

type processTree struct {
	job winproc.KillOnCloseJob
}

// configureProcessTree keeps the child suspended until its Job Object is ready.
// HideWindow keeps the child attached to a console while hiding a newly created
// console window, so console descendants can inherit it.
func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

func captureProcessTree(command *exec.Cmd) (processTree, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(command)
	if err != nil {
		return processTree{}, err
	}
	return processTree{job: job}, nil
}

func (tree processTree) kill(command *exec.Cmd) {
	if tree.job.Valid() {
		_ = tree.job.Close()
		return
	}
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}
