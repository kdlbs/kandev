//go:build linux || darwin

package launcher

import (
	"os/exec"
	"syscall"
)

func configureManagedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killManagedProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
