//go:build !windows

package shell

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureShellProcess(_ *exec.Cmd) {}

func killShellProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if killErr := p.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}
