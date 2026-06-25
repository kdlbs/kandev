//go:build windows

package shell

import (
	"errors"
	"os"
	"os/exec"
)

func configureShellProcess(_ *exec.Cmd) {}

func killShellProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
