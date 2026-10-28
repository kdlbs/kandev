package subproc

import (
	"os"
	"testing"
)

// The policy tests in this package describe the flag-on behavior; flag-off
// tests turn the mode off for their own duration.
func TestMain(m *testing.M) {
	SetManagedGitIsolation(true)
	os.Exit(m.Run())
}

func setIsolationForTest(t *testing.T, enabled bool) {
	t.Helper()
	previous := ManagedGitIsolation()
	SetManagedGitIsolation(enabled)
	t.Cleanup(func() { SetManagedGitIsolation(previous) })
}
