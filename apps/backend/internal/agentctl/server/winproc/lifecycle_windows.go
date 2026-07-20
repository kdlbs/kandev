//go:build windows

package winproc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillOnCloseJob owns a Windows Job Object configured to terminate all assigned
// processes when the handle is closed.
type KillOnCloseJob struct {
	state *killOnCloseJobState
}

type killOnCloseJobState struct {
	mu     sync.Mutex
	handle windows.Handle
}

// InstallKillOnCloseJobForSuspendedCommand assigns a suspended child process to
// a kill-on-close Job Object before resuming it. If job setup fails, the
// suspended child is terminated before it can create unowned descendants.
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
	return newKillOnCloseJob(job), nil
}

func InstallKillOnCloseJobForSuspendedProcess(pid int) (KillOnCloseJob, error) {
	job, err := createKillOnCloseJob()
	if err != nil {
		return KillOnCloseJob{}, errors.Join(err, terminateSuspendedProcess(pid))
	}
	if err := assignProcessToJob(job, pid); err != nil {
		_ = windows.CloseHandle(job)
		return KillOnCloseJob{}, errors.Join(
			err,
			terminateSuspendedProcess(pid),
		)
	}
	if err := ResumeSuspendedProcess(pid); err != nil {
		_ = windows.CloseHandle(job)
		return KillOnCloseJob{}, err
	}
	return newKillOnCloseJob(job), nil
}

func terminateSuspendedProcess(pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess(pid=%d) for suspended cleanup: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	if err := windows.TerminateProcess(process, 1); err != nil {
		return fmt.Errorf("TerminateProcess(pid=%d): %w", pid, err)
	}
	return nil
}

func (j KillOnCloseJob) Close() error {
	if j.state == nil {
		return nil
	}
	j.state.mu.Lock()
	defer j.state.mu.Unlock()
	if j.state.handle == 0 {
		return nil
	}
	if err := windows.CloseHandle(j.state.handle); err != nil {
		return err
	}
	j.state.handle = 0
	return nil
}

func (j KillOnCloseJob) RawHandle() uintptr {
	if j.state == nil {
		return 0
	}
	j.state.mu.Lock()
	defer j.state.mu.Unlock()
	return uintptr(j.state.handle)
}

// Valid reports whether this value refers to a job lifecycle, including one
// already reaped through another copy of the handle.
func (j KillOnCloseJob) Valid() bool {
	return j.state != nil
}

// TerminateAndWait terminates every process assigned to the job and waits
// until Windows reports that the job is empty before releasing its handle.
// On failure the handle remains owned so teardown can retry safely.
func (j KillOnCloseJob) TerminateAndWait(ctx context.Context) error {
	if j.state == nil {
		return nil
	}
	j.state.mu.Lock()
	defer j.state.mu.Unlock()
	if j.state.handle == 0 {
		return nil
	}

	active, err := activeJobProcesses(j.state.handle)
	if err != nil {
		return err
	}
	if active > 0 {
		if err := windows.TerminateJobObject(j.state.handle, 1); err != nil {
			return fmt.Errorf("TerminateJobObject: %w", err)
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for active > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for job processes: %w", ctx.Err())
		case <-ticker.C:
		}
		active, err = activeJobProcesses(j.state.handle)
		if err != nil {
			return err
		}
	}

	if err := windows.CloseHandle(j.state.handle); err != nil {
		return err
	}
	j.state.handle = 0
	return nil
}

type basicJobAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func newKillOnCloseJob(handle windows.Handle) KillOnCloseJob {
	return KillOnCloseJob{state: &killOnCloseJobState{handle: handle}}
}

func activeJobProcesses(job windows.Handle) (uint32, error) {
	var info basicJobAccountingInformation
	if err := windows.QueryInformationJobObject(
		job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	); err != nil {
		return 0, fmt.Errorf("QueryInformationJobObject: %w", err)
	}
	return info.ActiveProcesses, nil
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
