//go:build windows

package service

import "errors"

// sendOrphanReapSignal is never invoked on Windows: the platform snapshotter
// always fails with errOrphanReapUnsupportedPlatform first, so
// runOrphanReapPhase never reaches signal sending. This exists only so the
// package builds on Windows.
func sendOrphanReapSignal(int, orphanReapSignal) error {
	return errors.New("orphan reap: unsupported platform windows")
}

func isOrphanReapProcessGone(error) bool      { return false }
func isOrphanReapPermissionDenied(error) bool { return false }
