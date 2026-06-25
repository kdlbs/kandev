//go:build windows

package utility

import (
	"fmt"
	"os/exec"
	"syscall"
)

func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func terminateACPProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

func killACPProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

func acpProcessGroupAlive(_ int) bool {
	return false
}

func acpProcessGroupMissing(_ error) bool {
	return false
}
