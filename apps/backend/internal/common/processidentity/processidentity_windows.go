//go:build windows

package processidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

func Capture(pid int) (Identity, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Identity{}, err
	}
	defer windows.CloseHandle(process)
	creationTime, err := processCreationTime(process)
	if err != nil {
		return Identity{}, err
	}
	identity := Identity{PID: pid, BirthToken: fmt.Sprintf("windows:%d", creationTime)}
	if pid == os.Getpid() {
		name := takenWindowsJobName()
		if processInWindowsOwnedJob(process, name) {
			identity.WindowsJobName = name
		}
	}
	return identity, nil
}

func Inspect(identity Identity) (State, error) {
	if err := identity.Validate(); err != nil {
		return StateUnknown, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(identity.PID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return StateExited, nil
		}
		return StateUnknown, err
	}
	defer windows.CloseHandle(process)
	creationTime, exited, err := processTimes(process)
	if err != nil {
		return StateUnknown, err
	}
	want, err := parseWindowsBirthToken(identity.BirthToken)
	if err != nil {
		return StateUnknown, err
	}
	if creationTime != want {
		return StateReused, nil
	}
	if exited {
		return StateExited, nil
	}
	return StateAlive, nil
}

func TerminateOwnedSession(ctx context.Context, identity Identity, _ time.Duration) error {
	terminated, err := OwnedSessionTerminated(identity)
	if err != nil {
		return err
	}
	if terminated {
		return nil
	}
	state, err := Inspect(identity)
	if err != nil || !windowsOriginalOwnerGone(state) {
		return fmt.Errorf("refusing containment while the original owner is not confirmed exited: %w", ErrUnverifiableIdentity)
	}
	job, err := openWindowsOwnedJob(identity.WindowsJobName, windowsJobQuery|windowsJobTerminate)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(job) }()
	return winproc.TerminateJobAndWaitHandle(ctx, uintptr(job))
}

func processCreationTime(process windows.Handle) (uint64, error) {
	created, _, err := processTimes(process)
	return created, err
}

func processTimes(process windows.Handle) (uint64, bool, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return 0, false, err
	}
	return uint64(creation.Nanoseconds()), exit.LowDateTime != 0 || exit.HighDateTime != 0, nil
}

func parseWindowsBirthToken(token string) (uint64, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 || parts[0] != "windows" {
		return 0, ErrUnverifiableIdentity
	}
	created, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || created == 0 {
		return 0, ErrUnverifiableIdentity
	}
	return created, nil
}

const (
	windowsJobQuery     = 0x0004
	windowsJobTerminate = 0x0008
)

var openJobObjectW = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")
var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

type windowsJobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// OwnedSessionTerminated verifies the original birth identity and its launcher's
// Job Object. Legacy identities without job evidence remain fail-closed.
func OwnedSessionTerminated(identity Identity) (bool, error) {
	return windowsOwnedSessionTerminated(identity, Inspect, windowsOwnedJobActiveProcesses)
}

func windowsOwnedSessionTerminated(
	identity Identity,
	inspect func(Identity) (State, error),
	activeProcesses func(string) (uint32, error),
) (bool, error) {
	if err := identity.Validate(); err != nil {
		return false, err
	}
	if _, err := parseWindowsBirthToken(identity.BirthToken); err != nil {
		return false, err
	}
	state, err := inspect(identity)
	if err != nil {
		return false, err
	}
	if state == StateAlive {
		return false, nil
	}
	if !windowsOriginalOwnerGone(state) || !validWindowsJobName(identity.WindowsJobName) {
		return false, ErrUnverifiableIdentity
	}
	active, err := activeProcesses(identity.WindowsJobName)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		// A kill-on-close job is destroyed only after its members terminate.
		return true, nil
	}
	return err == nil && active == 0, err
}

// windowsOriginalOwnerGone reports whether Inspect proved that the original
// owner process no longer runs. StateReused also proves it: the PID now belongs
// to a process with a different creation time. The recorded job name, not the
// PID, then decides whether any of the owner's tree is still running, because
// the named job contains only the original owner and its descendants.
func windowsOriginalOwnerGone(state State) bool {
	return state == StateExited || state == StateReused
}

func openWindowsOwnedJob(name string, access uint32) (windows.Handle, error) {
	if !validWindowsJobName(name) {
		return 0, ErrUnverifiableIdentity
	}
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	handle, _, callErr := openJobObjectW.Call(uintptr(access), 0, uintptr(unsafe.Pointer(nameUTF16)))
	if handle == 0 {
		return 0, callErr
	}
	return windows.Handle(handle), nil
}

func windowsOwnedJobActiveProcesses(name string) (uint32, error) {
	job, err := openWindowsOwnedJob(name, windowsJobQuery)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(job) }()
	var info windowsJobAccounting
	if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0, err
	}
	return info.ActiveProcesses, nil
}

func processInWindowsOwnedJob(process windows.Handle, name string) bool {
	job, err := openWindowsOwnedJob(name, windowsJobQuery)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(job) }()
	var member int32
	ok, _, _ := isProcessInJob.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&member)))
	return ok != 0 && member != 0
}
