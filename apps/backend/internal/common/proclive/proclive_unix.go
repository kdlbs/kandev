//go:build !windows

package proclive

import "syscall"

// Alive reports whether pid is alive. Unix signal-zero checks distinguish a
// dead process from a process that exists but cannot be signaled by the caller.
func Alive(pid int64) (alive, known bool) {
	if pid <= 0 {
		return false, true
	}
	err := syscall.Kill(int(pid), 0)
	if err == nil || err == syscall.EPERM {
		return true, true
	}
	if err == syscall.ESRCH {
		return false, true
	}
	return false, false
}
