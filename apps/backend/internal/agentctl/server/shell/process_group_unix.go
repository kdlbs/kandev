//go:build !windows

package shell

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureShellProcess(_ *exec.Cmd) {}

type shellProcessLifecycleHandle struct{}

func installShellProcessLifecycle(_ *exec.Cmd) (shellProcessLifecycleHandle, error) {
	return shellProcessLifecycleHandle{}, nil
}

func reapShellProcessLifecycle(_ shellProcessLifecycleHandle) error { return nil }

func ownsShellProcessLifecycle(_ shellProcessLifecycleHandle) bool { return false }

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

func shellProcessGroupAlive(p *os.Process) bool {
	if p == nil || p.Pid <= 0 {
		return false
	}
	err := syscall.Kill(-p.Pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
