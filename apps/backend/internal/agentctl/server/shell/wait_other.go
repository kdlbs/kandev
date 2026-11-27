//go:build !windows

package shell

import "os/exec"

// waitShellCommand reaps a shell started by ptyexec, which started cmd itself.
func waitShellCommand(cmd *exec.Cmd) error { return cmd.Wait() }
