//go:build windows

package lifecycle

import "os"

// isLocalPIDAlive reports whether a process with the given pid currently exists
// on this host. On Windows os.FindProcess opens the process handle (via
// OpenProcess). Only ever called for local/standalone rows — never for a remote
// SSH pid (see RowProcessLiveness).
func isLocalPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	// Windows does not guarantee OpenProcess fails for a recently exited process
	// whose handle is still open elsewhere. This may briefly report alive after
	// exit, which is acceptable because Windows is not a server target.
	return true
}
