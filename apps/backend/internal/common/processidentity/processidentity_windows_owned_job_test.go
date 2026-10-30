//go:build windows

package processidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// exitedWindowsPID returns a PID that no live process holds, so Inspect reports
// the original owner as exited without starting or stopping any process.
func exitedWindowsPID(t *testing.T) int {
	t.Helper()
	for pid := 0x3FFFFFFC; pid > 0x3FFF0000; pid -= 4 {
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err == nil {
			_ = windows.CloseHandle(handle)
			continue
		}
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return pid
		}
	}
	t.Fatal("no unused PID found")
	return 0
}

// ownedIdentity builds the durable runtime ownership record shape, so the test
// reads the job name the same way recovery does.
func ownedIdentity(t *testing.T, pid int, jobName string) Identity {
	t.Helper()
	var identity Identity
	raw := fmt.Sprintf(`{"pid":%d,"birth_token":"windows:123","windows_job_name":%q}`, pid, jobName)
	if err := json.Unmarshal([]byte(raw), &identity); err != nil {
		t.Fatal(err)
	}
	return identity
}

func createEmptyNamedJob(t *testing.T, name string) {
	t.Helper()
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	job, err := windows.CreateJobObject(nil, nameUTF16)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(job) })
}

func TestWindowsExitedOwnerWithEmptyOwnedJobIsTerminated(t *testing.T) {
	name := `Local\kandev-agentctl-` + uuid.NewString()
	createEmptyNamedJob(t, name)
	identity := ownedIdentity(t, exitedWindowsPID(t), name)

	terminated, err := OwnedSessionTerminated(identity)
	if err != nil || !terminated {
		t.Fatalf("OwnedSessionTerminated(exited owner, empty owned job) = %v, %v; want true, nil", terminated, err)
	}
	if err := TerminateOwnedSession(context.Background(), identity, 0); err != nil {
		t.Fatalf("TerminateOwnedSession(exited owner, empty owned job) = %v; want nil", err)
	}
}

func TestWindowsExitedOwnerWithClosedOwnedJobIsTerminated(t *testing.T) {
	name := `Local\kandev-agentctl-` + uuid.NewString()
	identity := ownedIdentity(t, exitedWindowsPID(t), name)

	terminated, err := OwnedSessionTerminated(identity)
	if err != nil || !terminated {
		t.Fatalf("OwnedSessionTerminated(exited owner, closed kill-on-close job) = %v, %v; want true, nil", terminated, err)
	}
}

func TestWindowsOwnedSessionWithoutOwnedJobStaysFailClosed(t *testing.T) {
	pid := exitedWindowsPID(t)
	for _, jobName := range []string{"", `Local\foreign-job`, `Local\kandev-agentctl-not-a-uuid`} {
		terminated, err := OwnedSessionTerminated(ownedIdentity(t, pid, jobName))
		if terminated || !errors.Is(err, ErrUnverifiableIdentity) {
			t.Fatalf("OwnedSessionTerminated(job %q) = %v, %v; want false, ErrUnverifiableIdentity", jobName, terminated, err)
		}
		if err := TerminateOwnedSession(context.Background(), ownedIdentity(t, pid, jobName), 0); !errors.Is(err, ErrUnverifiableIdentity) {
			t.Fatalf("TerminateOwnedSession(job %q) = %v; want ErrUnverifiableIdentity", jobName, err)
		}
	}
}
