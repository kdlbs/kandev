package processidentity

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func resetTakenWindowsJobName(t *testing.T) {
	t.Helper()
	reset := func() {
		ownWindowsJobName.Lock()
		ownWindowsJobName.name, ownWindowsJobName.taken = "", false
		ownWindowsJobName.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestTakeWindowsJobNameFromEnvRemovesItFromDescendants(t *testing.T) {
	resetTakenWindowsJobName(t)
	outer := WindowsJobNamePrefix + "outer"
	t.Setenv(WindowsJobNameEnv, outer)

	TakeWindowsJobNameFromEnv()

	if _, ok := os.LookupEnv(WindowsJobNameEnv); ok {
		t.Fatal("job name is still in the process environment after it was taken")
	}
	for _, entry := range exec.Command("agent").Environ() {
		if strings.HasPrefix(strings.ToUpper(entry), WindowsJobNameEnv+"=") {
			t.Fatalf("a child command would inherit %q", entry)
		}
	}
	if got := takenWindowsJobName(); got != outer {
		t.Fatalf("takenWindowsJobName() = %q, want %q", got, outer)
	}

	t.Setenv(WindowsJobNameEnv, WindowsJobNamePrefix+"later")
	TakeWindowsJobNameFromEnv()
	if got := takenWindowsJobName(); got != outer {
		t.Fatalf("takenWindowsJobName() after a later value = %q, want the first %q", got, outer)
	}
}

func TestTakeWindowsJobNameFromEnvWithoutVariable(t *testing.T) {
	resetTakenWindowsJobName(t)
	t.Setenv(WindowsJobNameEnv, "")
	_ = os.Unsetenv(WindowsJobNameEnv)

	TakeWindowsJobNameFromEnv()

	if got := takenWindowsJobName(); got != "" {
		t.Fatalf("takenWindowsJobName() = %q, want empty", got)
	}
}
