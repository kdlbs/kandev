//go:build !windows

package acpdbg

import (
	"os/exec"
	"syscall"
)

// processTree carries no state outside Windows: the child is placed in its own
// process group at spawn time, so the whole group can be signalled by negating
// the leader's pid. See the Windows variant for why killing only the direct
// child is not enough.
type processTree struct{}

// configureProcessTree must be called before cmd.Start.
func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func captureProcessTree(_ *exec.Cmd) (processTree, error) { return processTree{}, nil }

func (processTree) kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// A negative pid targets the whole process group. If the group is gone (or
	// Setpgid did not take effect) fall back to the direct child.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
