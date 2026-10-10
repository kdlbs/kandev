//go:build !windows

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockRemoteHelperCache(cacheRoot string) (*os.File, error) {
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create remote helper cache: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(cacheRoot, remoteHelperCacheLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open remote helper cache lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock remote helper cache: %w", err)
	}
	return file, nil
}

func unlockRemoteHelperCache(file *os.File) error {
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil || closeErr != nil {
		return fmt.Errorf("unlock remote helper cache: %w", errors.Join(unlockErr, closeErr))
	}
	return nil
}
