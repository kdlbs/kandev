//go:build windows

package process

import (
	"errors"
	"os/exec"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func managedProcessExitDisposition(err error) (types.ManagedStartupExitDisposition, *int) {
	if err == nil {
		code := 0
		return types.ManagedStartupExitOrdinary, &code
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() < 0 {
		return types.ManagedStartupExitUnknown, nil
	}
	code := exitErr.ExitCode()
	return types.ManagedStartupExitOrdinary, &code
}
