//go:build !windows

package codexdbg

import (
	"os/exec"
	"syscall"
)

type processTree struct{}

func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func captureProcessTree(*exec.Cmd) (processTree, error) { return processTree{}, nil }

func (processTree) kill(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
		_ = command.Process.Kill()
	}
}
