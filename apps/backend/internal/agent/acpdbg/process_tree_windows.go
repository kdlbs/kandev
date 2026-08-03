//go:build windows

package acpdbg

import (
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

// processTree holds whatever the platform needs to kill a child *and* every
// process it spawned. Agents are usually launched through `npx`, which is a
// shim: npx -> node -> the agent's own node process. Killing only the direct
// child leaves the grandchildren alive holding the inherited stdout handle, so
// the read loop never sees EOF and Close blocks forever.
type processTree struct {
	job winproc.KillOnCloseJob
}

// configureProcessTree keeps the child suspended until its Job Object is ready.
func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

// captureProcessTree assigns the suspended child before resuming it. The
// helper terminates the child if containment or resume setup fails.
func captureProcessTree(cmd *exec.Cmd) (processTree, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return processTree{}, err
	}
	return processTree{job: job}, nil
}

func (t processTree) kill(cmd *exec.Cmd) {
	if t.job.Valid() {
		_ = t.job.Close()
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
