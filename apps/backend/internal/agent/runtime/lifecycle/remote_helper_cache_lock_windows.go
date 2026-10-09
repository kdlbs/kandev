//go:build windows

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func lockRemoteHelperCache(cacheRoot string) (*os.File, error) {
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create remote helper cache: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(cacheRoot, remoteHelperCacheLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open remote helper cache lock: %w", err)
	}
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{}); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock remote helper cache: %w", err)
	}
	return file, nil
}

func unlockRemoteHelperCache(file *os.File) error {
	unlockErr := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
	closeErr := file.Close()
	if unlockErr != nil || closeErr != nil {
		return fmt.Errorf("unlock remote helper cache: %w", errors.Join(unlockErr, closeErr))
	}
	return nil
}
