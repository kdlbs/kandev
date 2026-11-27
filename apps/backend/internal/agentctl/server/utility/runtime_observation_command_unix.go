//go:build !windows

package utility

import (
	"os/exec"
)

func configureRuntimeObservationCommand(_ *exec.Cmd, _ string, _ []string, _ []string) error {
	return nil
}
