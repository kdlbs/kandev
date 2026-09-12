//go:build !windows

package service

import "syscall"

// sendOrphanReapSignal sends sig to pid alone, never a process group.
func sendOrphanReapSignal(pid int, sig orphanReapSignal) error {
	var unixSig syscall.Signal
	switch sig {
	case orphanReapSigkill:
		unixSig = syscall.SIGKILL
	default:
		unixSig = syscall.SIGTERM
	}
	return syscall.Kill(pid, unixSig)
}

// isOrphanReapProcessGone reports whether the signal failed because the
// process no longer exists.
func isOrphanReapProcessGone(err error) bool {
	return err == syscall.ESRCH
}

// isOrphanReapPermissionDenied reports whether the signal failed because
// the system lacks permission.
func isOrphanReapPermissionDenied(err error) bool {
	return err == syscall.EPERM
}
