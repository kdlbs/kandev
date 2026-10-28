//go:build windows

package copilotplugins

import (
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

// commandTree carries the Job Object that contains the suspended child and
// every descendant it creates. npx is a shim (npx -> node -> CLI), so killing
// only the direct child would leave grandchildren holding inherited pipes.
type commandTree struct {
	job winproc.KillOnCloseJob
}

func prepareCommandTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

func captureCommandTree(cmd *exec.Cmd) (commandTree, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return commandTree{}, err
	}
	return commandTree{job: job}, nil
}

func (t commandTree) kill(cmd *exec.Cmd) {
	if t.job.Valid() {
		_ = t.job.Close()
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
