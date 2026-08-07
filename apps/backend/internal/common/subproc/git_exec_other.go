//go:build !linux && !darwin

package subproc

import "errors"

// ExecGit is unavailable on platforms that do not use the inherited-directory
// Git helper. The platform-specific caller does not invoke this stub.
func ExecGit(string, []string, []string) error {
	return errors.New("inherited Git execution is unsupported on this platform")
}
