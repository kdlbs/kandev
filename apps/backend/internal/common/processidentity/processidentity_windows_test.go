//go:build windows

package processidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/winproc"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWindowsOwnedSessionAliveIsNotTerminated(t *testing.T) {
	identity, err := Capture(os.Getpid())
	require.NoError(t, err)
	terminated, err := OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated)
}

func TestWindowsOwnedSessionNamedJobProvesTermination(t *testing.T) {
	name := `Local\kandev-agentctl-` + uuid.NewString()
	nameUTF16, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err)
	job, err := windows.CreateJobObject(nil, nameUTF16)
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(job) })
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	require.NoError(t, err)
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(process) })
	require.NoError(t, windows.AssignProcessToJobObject(job, process))
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, winproc.ResumeSuspendedProcess(cmd.Process.Pid))
	require.NoError(t, cmd.Wait())
	state, err := Inspect(identity)
	require.NoError(t, err)
	require.Equal(t, StateExited, state, "an open Windows handle must not make an exited owner appear live")
	// Read the same shape as the durable runtime ownership record.
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"windows_job_name":%q}`, name)), &identity))
	terminated, err := OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated)
}

func TestWindowsOwnedSessionMockProcessTable(t *testing.T) {
	name := WindowsJobNamePrefix + uuid.NewString()
	for _, tc := range []struct {
		name           string
		state          State
		jobName        string
		active         uint32
		jobErr         error
		wantTerminated bool
		wantErr        bool
		wantJobRead    bool
	}{
		{name: "live owner", state: StateAlive},
		{name: "empty owned job", state: StateExited, jobName: name, wantTerminated: true, wantJobRead: true},
		{name: "orphan in owned job", state: StateExited, jobName: name, active: 1, wantJobRead: true},
		{name: "closed kill-on-close job", state: StateExited, jobName: name, jobErr: windows.ERROR_FILE_NOT_FOUND, wantTerminated: true, wantJobRead: true},
		{name: "PID reused with empty owned job", state: StateReused, jobName: name, wantTerminated: true, wantJobRead: true},
		{name: "PID reused with closed owned job", state: StateReused, jobName: name, jobErr: windows.ERROR_FILE_NOT_FOUND, wantTerminated: true, wantJobRead: true},
		{name: "PID reused with orphan in owned job", state: StateReused, jobName: name, active: 1, wantJobRead: true},
		{name: "PID reused without owned job", state: StateReused, wantErr: true},
		{name: "unknown process", state: StateUnknown, jobName: name, wantErr: true},
		{name: "legacy unknown job", state: StateExited, wantErr: true},
		{name: "foreign job", state: StateExited, jobName: "foreign", wantErr: true},
		{name: "job unreadable", state: StateExited, jobName: name, jobErr: windows.ERROR_ACCESS_DENIED, wantErr: true, wantJobRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := Identity{PID: 42, BirthToken: "windows:123", WindowsJobName: tc.jobName}
			table := map[int]State{42: tc.state}
			jobRead := false
			terminated, err := windowsOwnedSessionTerminated(identity, func(got Identity) (State, error) {
				require.Equal(t, identity, got)
				return table[got.PID], nil
			}, func(got string) (uint32, error) {
				jobRead = true
				require.Equal(t, name, got)
				return tc.active, tc.jobErr
			})
			require.Equal(t, tc.wantErr, err != nil)
			require.Equal(t, tc.wantTerminated, terminated)
			require.Equal(t, tc.wantJobRead, jobRead)
		})
	}
}

func TestWindowsContainmentRejectsUnknownAndLiveProcesses(t *testing.T) {
	identity, err := Capture(os.Getpid())
	require.NoError(t, err)
	require.Error(t, TerminateOwnedSession(context.Background(), identity, 0))
	identity.BirthToken = "windows:1"
	require.ErrorIs(t, TerminateOwnedSession(context.Background(), identity, 0), ErrUnverifiableIdentity)
	require.True(t, errors.Is(TerminateOwnedSession(context.Background(), Identity{PID: os.Getpid()}, 0), ErrUnverifiableIdentity))
}

func TestWindowsCaptureIgnoresJobNameLeftInEnvironment(t *testing.T) {
	resetTakenWindowsJobName(t)
	name := WindowsJobNamePrefix + uuid.NewString()
	createEmptyNamedJob(t, name)
	t.Setenv(WindowsJobNameEnv, name)

	identity, err := Capture(os.Getpid())
	require.NoError(t, err)
	require.Empty(t, identity.WindowsJobName, "Capture must use only the name taken at startup")
}

func TestWindowsReusedOwnerPIDUsesOwnedJobEvidence(t *testing.T) {
	// The current PID with another birth time is a reused PID: the original
	// owner is gone and this process is not in the named job.
	reused := func(jobName string) Identity {
		return Identity{PID: os.Getpid(), BirthToken: "windows:1", WindowsJobName: jobName}
	}
	state, err := Inspect(reused(""))
	require.NoError(t, err)
	require.Equal(t, StateReused, state)

	name := WindowsJobNamePrefix + uuid.NewString()
	createEmptyNamedJob(t, name)
	terminated, err := OwnedSessionTerminated(reused(name))
	require.NoError(t, err)
	require.True(t, terminated, "an empty owned job proves termination after PID reuse")
	require.NoError(t, TerminateOwnedSession(context.Background(), reused(name), 0))

	closed := WindowsJobNamePrefix + uuid.NewString()
	terminated, err = OwnedSessionTerminated(reused(closed))
	require.NoError(t, err)
	require.True(t, terminated, "a closed kill-on-close job proves termination after PID reuse")

	_, err = OwnedSessionTerminated(reused(""))
	require.ErrorIs(t, err, ErrUnverifiableIdentity)
}

type windowsOwnedJobHelperResult struct {
	Identity Identity `json:"identity"`
	ChildPID int      `json:"child_pid"`
}

func TestWindowsNamedJobContainsOrphanAfterLeaderExit(t *testing.T) {
	name := WindowsJobNamePrefix + uuid.NewString()
	output := filepath.Join(t.TempDir(), "owned-job.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsOwnedJobHelper$")
	cmd.Env = append(os.Environ(), WindowsJobNameEnv+"="+name,
		"KANDEV_WINDOWS_OWNED_JOB_HELPER=root", "KANDEV_WINDOWS_OWNED_JOB_OUTPUT="+output)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	identity.WindowsJobName = name
	job, err := winproc.InstallNamedKillOnCloseJobForSuspendedProcess(cmd.Process.Pid, name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = job.Close() })
	require.NoError(t, cmd.Wait())
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	var result windowsOwnedJobHelperResult
	require.NoError(t, json.Unmarshal(data, &result))
	require.Equal(t, identity, result.Identity, "child must authenticate the inherited job with kernel membership")
	childIdentity, err := Capture(result.ChildPID)
	require.NoError(t, err)
	terminated, err := OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated, "a surviving member must block termination proof")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, TerminateOwnedSession(ctx, identity, 0))
	terminated, err = OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated)
	state, err := Inspect(childIdentity)
	require.NoError(t, err)
	require.Equal(t, StateExited, state)
}

func TestWindowsOwnedJobHelper(t *testing.T) {
	switch os.Getenv("KANDEV_WINDOWS_OWNED_JOB_HELPER") {
	case "leaf":
		timer := time.NewTimer(time.Hour)
		defer timer.Stop()
		<-timer.C
	case "root":
		TakeWindowsJobNameFromEnv()
		identity, err := Capture(os.Getpid())
		require.NoError(t, err)
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsOwnedJobHelper$")
		child.Env = append(os.Environ(), "KANDEV_WINDOWS_OWNED_JOB_HELPER=leaf")
		child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		require.NoError(t, child.Start())
		result := windowsOwnedJobHelperResult{Identity: identity, ChildPID: child.Process.Pid}
		data, err := json.Marshal(result)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(os.Getenv("KANDEV_WINDOWS_OWNED_JOB_OUTPUT"), data, 0600))
	}
}
