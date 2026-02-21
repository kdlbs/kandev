//go:build windows

package adapter

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup kills the entire process tree for the given PID.
func killProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}
