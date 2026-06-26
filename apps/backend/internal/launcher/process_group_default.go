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

func terminateManagedProcessGroup(_ int) error {
	return errors.New("process groups are not supported on this platform")
}
