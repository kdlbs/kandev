package worktree

import (
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

// The tests in this package describe the flag-on managed Git behavior.
func init() { subproc.SetManagedGitIsolation(true) }

// disableManagedGitIsolation runs a test against a scripted fake Git that
// understands only the commands the test is about.
func disableManagedGitIsolation(t *testing.T) {
	t.Helper()
	subproc.SetManagedGitIsolation(false)
	t.Cleanup(func() { subproc.SetManagedGitIsolation(true) })
}
