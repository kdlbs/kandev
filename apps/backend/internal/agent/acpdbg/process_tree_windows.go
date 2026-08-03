//go:build windows

package acpdbg

import (
	"os/exec"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
)

// processTree holds whatever the platform needs to kill a child *and* every
// process it spawned. Agents are usually launched through `npx`, which is a
// shim: npx -> node -> the agent's own node process. Killing only the direct
// child leaves the grandchildren alive holding the inherited stdout handle, so
// the read loop never sees EOF and Close blocks forever.
type processTree struct {
	job winproc.KillOnCloseJob
}

// configureProcessTree is a no-op on Windows; containment is established after
// the child starts, by assigning it to a Job Object.
func configureProcessTree(_ *exec.Cmd) {}

// captureProcessTree must be called after cmd.Start, because the Job Object is
// assigned by pid. A failure here is not fatal: kill falls back to terminating
// the direct child, which is what the code did before.
func captureProcessTree(cmd *exec.Cmd) processTree {
	job, err := winproc.InstallKillOnCloseJobForCommand(cmd)
	if err != nil {
		return processTree{}
	}
	return processTree{job: job}
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
