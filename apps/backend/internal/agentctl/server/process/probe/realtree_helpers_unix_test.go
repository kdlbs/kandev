//go:build unix

package probe

import (
	"os/exec"
	"syscall"
)

// withOwnProcessGroup puts cmd in its own process group, reproducing the
// shape §L measured for a detached background shell.
func withOwnProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
