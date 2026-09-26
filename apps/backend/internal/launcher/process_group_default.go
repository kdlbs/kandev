//go:build !linux && !darwin && !windows

package launcher

import (
	"errors"
	"os/exec"
)

func ignoreBrokenPipeSignal() {
}

func configureManagedProcess(_ *exec.Cmd) {
}

func killManagedProcessGroup(_ int) error {
	return errors.New("process groups are not supported on this platform")
}

func managedProcessGracefulScope(_ bool) gracefulSignalScope {
	return gracefulSignalUnsupported
}

func terminateManagedProcess(pid, _ int, _ bool) error {
	return terminateManagedProcessGroup(pid)
}

func terminateManagedProcessGroup(_ int) error {
	return errors.New("process groups are not supported on this platform")
}

func isManagedProcessTarget(_, _ int) bool {
	return false
}

func managedProcessGroupCleanupSupported() bool {
	return false
}
