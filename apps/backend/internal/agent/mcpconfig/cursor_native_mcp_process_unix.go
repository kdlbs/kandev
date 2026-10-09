//go:build unix

package mcpconfig

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

func prepareNativeMCPProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	attr := *cmd.SysProcAttr
	if attr.Setctty || attr.Foreground || attr.Ctty != 0 {
		return errors.New("native MCP command requested a controlling terminal")
	}
	attr.Setsid = true
	attr.Setpgid = false
	attr.Pgid = 0
	cmd.SysProcAttr = &attr
	return nil
}

func attachNativeMCPProcess(cmd *exec.Cmd) (nativeMCPProcessGuard, error) {
	if cmd == nil || cmd.Process == nil {
		return nativeMCPProcessGuard{}, errors.New("native MCP process did not start")
	}
	pid := cmd.Process.Pid
	return nativeMCPProcessGuard{terminate: func(context.Context) error {
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}}, nil
}
