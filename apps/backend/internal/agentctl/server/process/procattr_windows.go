//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
// On Windows, we use CREATE_NEW_PROCESS_GROUP flag.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup kills the entire process tree for the given PID.
// On Windows, we use taskkill with /T flag to kill the process tree.
func killProcessGroup(pid int) error {
	// taskkill /F /T /PID <pid>
	// /F = force kill
	// /T = kill child processes (tree)
	// /PID = process ID
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

