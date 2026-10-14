//go:build windows

package mcpconfig

import (
	"context"
	"errors"
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

func prepareNativeMCPProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	attr := *cmd.SysProcAttr
	if attr.CreationFlags&windows.CREATE_NEW_CONSOLE != 0 {
		return errors.New("native MCP command requested a new console")
	}
	// HideWindow keeps the child attached to a console while hiding a newly
	// created console window. Console descendants can then inherit that console.
	attr.HideWindow = true
	attr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED
	cmd.SysProcAttr = &attr
	return nil
}

func attachNativeMCPProcess(cmd *exec.Cmd) (nativeMCPProcessGuard, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return nativeMCPProcessGuard{}, err
	}
	return nativeMCPProcessGuard{terminate: func(ctx context.Context) error {
		if err := job.TerminateAndWait(ctx); err != nil {
			return err
		}
		return job.Close()
	}}, nil
}
