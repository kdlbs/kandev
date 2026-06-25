//go:build windows

package utility

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func terminateACPProcessGroup(pid int) error {
	return runTaskkill("/T", "/PID", fmt.Sprintf("%d", pid))
}

func killACPProcessGroup(pid int) error {
	return runTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", pid))
}

func runTaskkill(args ...string) error {
	output, err := exec.Command("taskkill", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}

func acpProcessGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(output))
	if strings.Contains(text, "no tasks") {
		return false
	}
	return strings.Contains(text, strconv.Itoa(pid))
}

func acpProcessGroupMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not be found") ||
		strings.Contains(msg, "no running instance") ||
		strings.Contains(msg, "not running")
}
