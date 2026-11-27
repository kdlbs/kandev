package mcpconfig

import (
	"context"
	"os/exec"
	"time"
)

const nativeMCPWaitDelay = 500 * time.Millisecond

type nativeMCPProcessGuard struct {
	terminate func(context.Context) error
}

func (g nativeMCPProcessGuard) Terminate(ctx context.Context) error {
	if g.terminate == nil {
		return nil
	}
	return g.terminate(ctx)
}

func (g nativeMCPProcessGuard) Cleanup(ctx context.Context) error {
	return g.Terminate(ctx)
}

func prepareNativeMCPCommand(cmd *exec.Cmd) error {
	return prepareNativeMCPProcess(cmd)
}

func attachNativeMCPCommand(cmd *exec.Cmd) (nativeMCPProcessGuard, error) {
	return attachNativeMCPProcess(cmd)
}
