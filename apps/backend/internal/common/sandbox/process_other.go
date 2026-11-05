//go:build !unix

package sandbox

import "os/exec"

func ConfigureProcess(_ *exec.Cmd) error { return ErrInvalidPolicy }
