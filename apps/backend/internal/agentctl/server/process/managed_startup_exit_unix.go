//go:build !windows

package process

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func managedProcessExitDisposition(err error) (types.ManagedStartupExitDisposition, *int) {
	if err == nil {
		code := 0
		return types.ManagedStartupExitOrdinary, &code
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return types.ManagedStartupExitUnknown, nil
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return types.ManagedStartupExitUnknown, nil
	}
	if status.Signaled() {
		return types.ManagedStartupExitSignal, nil
	}
	if !status.Exited() {
		return types.ManagedStartupExitUnknown, nil
	}
	code := status.ExitStatus()
	return types.ManagedStartupExitOrdinary, &code
}
