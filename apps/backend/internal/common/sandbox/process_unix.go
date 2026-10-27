//go:build unix

package sandbox

import (
	"os/exec"
	"syscall"
)

// ConfigureProcess gives the supervisor a private session for verifiable cleanup.
func ConfigureProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = false
	cmd.SysProcAttr.Setsid = true
	return nil
}
