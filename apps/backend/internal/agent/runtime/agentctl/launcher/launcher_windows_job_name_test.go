package launcher

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/processidentity"
)

func TestNewWindowsJobNameOnlyOnWindows(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		if got := newWindowsJobName(goos); got != "" {
			t.Fatalf("newWindowsJobName(%q) = %q, want empty", goos, got)
		}
	}
	got := newWindowsJobName("windows")
	suffix, ok := strings.CutPrefix(got, processidentity.WindowsJobNamePrefix)
	if !ok {
		t.Fatalf("newWindowsJobName(windows) = %q, want prefix %q", got, processidentity.WindowsJobNamePrefix)
	}
	if _, err := uuid.Parse(suffix); err != nil {
		t.Fatalf("newWindowsJobName(windows) = %q, want a UUID suffix: %v", got, err)
	}
}

func TestWithWindowsJobNameReplacesInheritedValue(t *testing.T) {
	inherited := []string{
		"PATH=x",
		processidentity.WindowsJobNameEnv + "=outer",
		strings.ToLower(processidentity.WindowsJobNameEnv) + "=outer-lower",
	}

	without := withWindowsJobName(inherited, "")
	if strings.Join(without, "\n") != "PATH=x" {
		t.Fatalf("withWindowsJobName(no job) = %v, want only PATH=x", without)
	}

	with := withWindowsJobName(inherited, "inner")
	want := []string{"PATH=x", processidentity.WindowsJobNameEnv + "=inner"}
	if strings.Join(with, "\n") != strings.Join(want, "\n") {
		t.Fatalf("withWindowsJobName(inner) = %v, want %v", with, want)
	}
}

func TestStartedProcessIdentityCaptureFailureKeepsOnlyJobName(t *testing.T) {
	previous := captureProcessIdentity
	t.Cleanup(func() { captureProcessIdentity = previous })
	captureErr := errors.New("capture failed")
	captureProcessIdentity = func(pid int) (processidentity.Identity, error) {
		return processidentity.Identity{PID: pid, BirthToken: "partial"}, captureErr
	}

	identity, err := startedProcessIdentity(42, "job")
	if !errors.Is(err, captureErr) {
		t.Fatalf("startedProcessIdentity error = %v, want %v", err, captureErr)
	}
	if identity != (processidentity.Identity{WindowsJobName: "job"}) {
		t.Fatalf("startedProcessIdentity(capture failure) = %+v, want only the job name", identity)
	}
	if verr := identity.Validate(); !errors.Is(verr, processidentity.ErrUnverifiableIdentity) {
		t.Fatalf("Validate() = %v, want ErrUnverifiableIdentity so cleanup fails closed", verr)
	}

	captureProcessIdentity = func(pid int) (processidentity.Identity, error) {
		return processidentity.Identity{PID: pid, BirthToken: "windows:1"}, nil
	}
	identity, err = startedProcessIdentity(42, "job")
	want := processidentity.Identity{PID: 42, BirthToken: "windows:1", WindowsJobName: "job"}
	if err != nil || identity != want {
		t.Fatalf("startedProcessIdentity(success) = %+v, %v; want %+v, nil", identity, err, want)
	}
}
