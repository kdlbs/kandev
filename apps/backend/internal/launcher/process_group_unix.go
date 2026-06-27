//go:build linux || darwin

package launcher

import (
	"os/exec"
	"os/signal"
	"syscall"
)

func ignoreBrokenPipeSignal() {
	signal.Ignore(syscall.SIGPIPE)
}

func configureManagedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killManagedProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func terminateManagedProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}
