//go:build !windows

package lifecycle

import (
	"errors"
	"os"
	"syscall"
)

// isLocalPIDAlive reports whether a process with the given pid currently exists
// on this host. It sends signal 0, which performs existence/permission checking
// without delivering a signal: a nil error means the process is alive, ESRCH
// means it is gone, and EPERM means it exists but is owned by another user
// (still alive). Only ever called for local/standalone rows — never for a remote
// SSH pid (see RowProcessLiveness).
func isLocalPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// The process exists but we lack permission to signal it — still alive.
	return errors.Is(err, syscall.EPERM)
}
