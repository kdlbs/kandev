//go:build !windows

package copilotplugins

import (
	"os/exec"
	"syscall"
)

type commandTree struct{}

func prepareCommandTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func captureCommandTree(_ *exec.Cmd) (commandTree, error) { return commandTree{}, nil }

func (commandTree) kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// A negative pid targets the whole process group. If the group is gone (or
	// Setpgid did not take effect) fall back to the direct child.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
