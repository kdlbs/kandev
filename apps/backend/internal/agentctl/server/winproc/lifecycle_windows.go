//go:build windows

package winproc

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillOnCloseJob owns a Windows Job Object configured to terminate all assigned
// processes when the handle is closed.
type KillOnCloseJob struct {
	handle windows.Handle
}

// InstallKillOnCloseJobForSuspendedCommand assigns a suspended child process to
// a kill-on-close Job Object before resuming it. If job setup fails, the child
// is still resumed so callers can fall back to explicit taskkill cleanup.
func InstallKillOnCloseJobForSuspendedCommand(cmd *exec.Cmd) (KillOnCloseJob, error) {
	if cmd == nil || cmd.Process == nil {
		return KillOnCloseJob{}, fmt.Errorf("process not started")
	}
	return InstallKillOnCloseJobForSuspendedProcess(cmd.Process.Pid)
}

// InstallKillOnCloseJobForCommand assigns an already-running child process to
// a kill-on-close Job Object. Use the suspended variant when the child must not
// execute before job assignment.
func InstallKillOnCloseJobForCommand(cmd *exec.Cmd) (KillOnCloseJob, error) {
	if cmd == nil || cmd.Process == nil {
		return KillOnCloseJob{}, fmt.Errorf("process not started")
	}
	return InstallKillOnCloseJobForProcess(cmd.Process.Pid)
}

func InstallKillOnCloseJobForProcess(pid int) (KillOnCloseJob, error) {
	job, err := createKillOnCloseJob()
	if err != nil {
		return KillOnCloseJob{}, err
	}
	if err := assignProcessToJob(job, pid); err != nil {
		_ = windows.CloseHandle(job)
		return KillOnCloseJob{}, err
	}
	return KillOnCloseJob{handle: job}, nil
}

func InstallKillOnCloseJobForSuspendedProcess(pid int) (KillOnCloseJob, error) {
	job, err := createKillOnCloseJob()
	if err != nil {
		return KillOnCloseJob{}, errors.Join(err, ResumeSuspendedProcess(pid))
	}
	if err := assignProcessToJob(job, pid); err != nil {
		_ = windows.CloseHandle(job)
		return KillOnCloseJob{}, errors.Join(
			err,
			ResumeSuspendedProcess(pid),
		)
	}
	if err := ResumeSuspendedProcess(pid); err != nil {
		_ = windows.CloseHandle(job)
		return KillOnCloseJob{}, err
	}
	return KillOnCloseJob{handle: job}, nil
}

func (j KillOnCloseJob) Close() error {
	if j.handle == 0 {
		return nil
	}
	return windows.CloseHandle(j.handle)
}

func (j KillOnCloseJob) RawHandle() uintptr {
	return uintptr(j.handle)
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

func assignProcessToJob(job windows.Handle, pid int) error {
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("OpenProcess(pid=%d): %w", pid, err)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}

func ResumeSuspendedProcess(pid int) error {
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
			if err := resumeThread(entry.ThreadID); err != nil {
				return err
			}
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

func resumeThread(threadID uint32) error {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, threadID)
	if err != nil {
		return fmt.Errorf("OpenThread(thread_id=%d): %w", threadID, err)
	}
	defer windows.CloseHandle(thread)
	if _, err := windows.ResumeThread(thread); err != nil {
		return fmt.Errorf("ResumeThread(thread_id=%d): %w", threadID, err)
	}
	return nil
}

func RunTaskkill(args ...string) error {
	output, err := exec.Command("taskkill", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	if IsTaskkillMissing(msg) {
		return syscall.ESRCH
	}
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}

func IsTaskkillMissing(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "not be found") ||
		strings.Contains(lower, "no running instance")
}
