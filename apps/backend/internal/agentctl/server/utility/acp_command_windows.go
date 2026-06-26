//go:build windows

package utility

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

// setACPCommandProcAttr starts ACP utility commands suspended so the cleanup
// lifecycle can attach a kill-on-close Job Object before the command runs.
func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

type acpCommandLifecycleHandle struct {
	job winproc.KillOnCloseJob
}

func installACPCommandLifecycle(cmd *exec.Cmd) (acpCommandLifecycleHandle, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return acpCommandLifecycleHandle{}, err
	}
	return acpCommandLifecycleHandle{job: job}, nil
}

func releaseACPCommandLifecycle(lifecycle acpCommandLifecycleHandle) {
	_ = lifecycle.job.Close()
}

func terminateACPProcessGroup(pid int) error {
	return runTaskkill("/T", "/PID", fmt.Sprintf("%d", pid))
}

func killACPProcessGroup(pid int) error {
	return runTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", pid))
}

func runTaskkill(args ...string) error {
	return winproc.RunTaskkill(args...)
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
