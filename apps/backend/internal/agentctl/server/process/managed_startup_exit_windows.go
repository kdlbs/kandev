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
	if isWindowsNTStatusExceptionExitCode(uint32(code)) {
		return types.ManagedStartupExitUnknown, nil
	}
	return types.ManagedStartupExitOrdinary, &code
}

func isWindowsNTStatusExceptionExitCode(code uint32) bool {
	return uint32(code)&0xc0000000 == 0xc0000000
}
