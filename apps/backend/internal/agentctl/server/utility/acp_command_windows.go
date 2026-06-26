//go:build windows

package utility

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func setACPCommandProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

type acpCommandLifecycleHandle struct {
	handle windows.Handle
}

func installACPCommandLifecycle(cmd *exec.Cmd) (acpCommandLifecycleHandle, error) {
	if cmd == nil || cmd.Process == nil {
		return acpCommandLifecycleHandle{}, fmt.Errorf("process not started")
	}
	job, err := createACPKillOnCloseJob()
	if err != nil {
		return acpCommandLifecycleHandle{}, err
	}
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return acpCommandLifecycleHandle{}, fmt.Errorf("OpenProcess(pid=%d): %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		_ = windows.CloseHandle(job)
		return acpCommandLifecycleHandle{}, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return acpCommandLifecycleHandle{handle: job}, nil
}

func releaseACPCommandLifecycle(lifecycle acpCommandLifecycleHandle) {
	if lifecycle.handle != 0 {
		_ = windows.CloseHandle(lifecycle.handle)
	}
}

func createACPKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return job, nil
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
