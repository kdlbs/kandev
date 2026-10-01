//go:build !unix && !windows

package mcpconfig

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

func prepareNativeMCPProcess(*exec.Cmd) error { return nil }

func attachNativeMCPProcess(cmd *exec.Cmd) (nativeMCPProcessGuard, error) {
	if cmd == nil || cmd.Process == nil {
		return nativeMCPProcessGuard{}, errors.New("native MCP process did not start")
	}
	return nativeMCPProcessGuard{terminate: func(context.Context) error {
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}}, nil
}
