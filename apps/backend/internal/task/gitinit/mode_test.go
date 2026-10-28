package gitinit

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

// The policy tests in this package describe the flag-on behavior.
func TestMain(m *testing.M) {
	subproc.SetManagedGitIsolation(true)
	os.Exit(m.Run())
}
