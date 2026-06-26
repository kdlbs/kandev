//go:build windows

package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func configureShellProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killShellProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := runShellTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", p.Pid)); err != nil {
		if killErr := p.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}

func runShellTaskkill(args ...string) error {
	output, err := exec.Command("taskkill", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}
