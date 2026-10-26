package lifecycle

import "github.com/kandev/kandev/internal/common/subproc"

// The tests in this package describe the flag-on managed Git behavior.
func init() { subproc.SetManagedGitIsolation(true) }
