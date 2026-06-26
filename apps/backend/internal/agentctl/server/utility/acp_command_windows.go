//go:build windows

package utility

import (
	"errors"
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
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

type acpCommandLifecycleHandle struct {
	handle windows.Handle
}

func installACPCommandLifecycle(cmd *exec.Cmd) (acpCommandLifecycleHandle, error) {
	if cmd == nil || cmd.Process == nil {
		return acpCommandLifecycleHandle{}, fmt.Errorf("process not started")
	}
	pid := cmd.Process.Pid
	job, err := createACPKillOnCloseJob()
	if err != nil {
		return acpCommandLifecycleHandle{}, errors.Join(err, resumeACPCommandProcess(pid))
	}
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return acpCommandLifecycleHandle{}, errors.Join(
			fmt.Errorf("OpenProcess(pid=%d): %w", pid, err),
			resumeACPCommandProcess(pid),
		)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		_ = windows.CloseHandle(job)
		return acpCommandLifecycleHandle{}, errors.Join(
			fmt.Errorf("AssignProcessToJobObject: %w", err),
			resumeACPCommandProcess(pid),
		)
	}
	if err := resumeACPCommandProcess(pid); err != nil {
		_ = windows.CloseHandle(job)
		return acpCommandLifecycleHandle{}, err
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

func resumeACPCommandProcess(pid int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return fmt.Errorf("Thread32First: %w", err)
	}

	resumed := 0
	for {
		if entry.OwnerProcessID == uint32(pid) {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return fmt.Errorf("OpenThread(thread_id=%d): %w", entry.ThreadID, err)
			}
			if _, err := windows.ResumeThread(thread); err != nil {
				_ = windows.CloseHandle(thread)
				return fmt.Errorf("ResumeThread(thread_id=%d): %w", entry.ThreadID, err)
			}
			_ = windows.CloseHandle(thread)
			resumed++
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return fmt.Errorf("Thread32Next: %w", err)
		}
	}
	if resumed == 0 {
		return fmt.Errorf("no threads found for pid %d", pid)
	}
	return nil
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
