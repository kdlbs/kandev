//go:build !darwin

package launcher

import "errors"

func replaceServiceShell(serviceShellInvocation) error {
	return errors.New("service login shells are supported only on macOS")
}
