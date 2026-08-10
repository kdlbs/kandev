//go:build !windows

package tempartifacts

import "syscall"

func processAlive(pid int64) (alive, known bool) {
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
