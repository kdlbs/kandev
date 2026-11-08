package managedruntime

import (
	"context"
	"errors"
	"os"
	"time"
)

const repairLockPollInterval = 50 * time.Millisecond

var (
	errRepairDirectoryPermissions = errors.New("managed runtime repair directory permissions are not private")
	errRepairLockHandle           = errors.New("managed runtime repair lock path is not a regular file")
)

// acquireRepairContextLock waits for the per-context cross-process lock while
// preserving the caller's cancellation and deadline.
func acquireRepairContextLock(ctx context.Context, file *os.File) error {
	if file == nil {
		return errors.New("managed runtime repair lock is unavailable")
	}
	for {
		locked, err := tryLockRepairContext(file)
		if err != nil {
			return err
		}
		if locked {
			return nil
		}
		timer := time.NewTimer(repairLockPollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}
