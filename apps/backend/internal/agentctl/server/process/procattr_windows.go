//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// setProcGroup configures the command to run in its own process group.
// On Windows, we use CREATE_NEW_PROCESS_GROUP flag.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

type processLifecycleHandle struct {
	handle windows.Handle
}

func installProcessLifecycle(cmd *exec.Cmd) (processLifecycleHandle, error) {
	if cmd == nil || cmd.Process == nil {
		return processLifecycleHandle{}, fmt.Errorf("process not started")
	}
	job, err := createKillOnCloseJob()
	if err != nil {
		return processLifecycleHandle{}, err
	}
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return processLifecycleHandle{}, fmt.Errorf("OpenProcess(pid=%d): %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		_ = windows.CloseHandle(job)
		return processLifecycleHandle{}, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return processLifecycleHandle{handle: job}, nil
}

func releaseProcessLifecycle(lifecycle processLifecycleHandle) {
	if lifecycle.handle != 0 {
		_ = windows.CloseHandle(lifecycle.handle)
	}
}

func createKillOnCloseJob() (windows.Handle, error) {
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

// terminateProcessGroup sends a graceful shutdown signal to the process tree.
// Without /F, taskkill sends WM_CLOSE to console and windowed apps, which is
// the closest Windows equivalent of SIGTERM.
func terminateProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

func processGroupAlive(_ int) bool {
	// taskkill /T is the authoritative process-tree operation on Windows.
	// There is no Unix-style process group to poll here.
	return false
}

func isProcessGroupMissing(_ error) bool {
	return false
}
