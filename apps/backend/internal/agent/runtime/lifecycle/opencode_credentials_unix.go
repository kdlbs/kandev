//go:build !windows

package lifecycle

import (
	"os/exec"
	"sync"
	"syscall"
)

func configureCredentialExportProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func captureCredentialExportProcess(cmd *exec.Cmd) (func(), error) {
	return sync.OnceFunc(func() {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			_ = cmd.Process.Kill()
		}
	}), nil
}
