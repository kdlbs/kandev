//go:build !darwin && !linux

package service

import "context"

// otherOrphanReapHost covers every platform without a defined detection
// mechanism, chiefly Windows (AC-TASKS-ORPHAN-REAP-007.4: the reap phase is a
// recorded no-op there).
type otherOrphanReapHost struct{}

func defaultOrphanReapHostSnapshotter() orphanReapHostSnapshotter { return otherOrphanReapHost{} }
func defaultOrphanReapVerifier() orphanReapVerifier               { return otherOrphanReapHost{} }

func (otherOrphanReapHost) Snapshot(context.Context) ([]hostProcess, error) {
	return nil, errOrphanReapUnsupportedPlatform
}

func (otherOrphanReapHost) VerifyCwd(context.Context, int) (string, error) {
	return "", errOrphanReapUnsupportedPlatform
}
