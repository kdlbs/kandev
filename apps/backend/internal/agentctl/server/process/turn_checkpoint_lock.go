package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func acquireTurnCheckpointLock(ctx context.Context, gitDir string) (func(), error) {
	lockPath := filepath.Join(gitDir, "kandev-turn-changes.capture.lock")
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		unlock, acquired, err := tryCreateTurnCheckpointLock(lockPath)
		if err != nil {
			return nil, err
		}
		if acquired {
			return unlock, nil
		}
		if _, err := removeStaleTurnCheckpointLock(lockPath, time.Now()); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func tryCreateTurnCheckpointLock(lockPath string) (func(), bool, error) {
	file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	_, writeErr := fmt.Fprintf(file, "%s %d\n", strconv.Itoa(os.Getpid()), time.Now().UnixNano())
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(lockPath)
		return nil, false, err
	}
	return func() { _ = os.Remove(lockPath) }, true, nil
}

func removeStaleTurnCheckpointLock(lockPath string, now time.Time) (bool, error) {
	info, err := os.Lstat(lockPath)
	if errors.Is(err, os.ErrNotExist) || err == nil && (now.Sub(info.ModTime()) <= turnCheckpointLockStale || !info.Mode().IsRegular()) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := os.Lstat(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(info, current) || !info.ModTime().Equal(current.ModTime()) {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func removeStaleTurnCheckpointIndexes(gitDir string, now time.Time) error {
	paths, err := filepath.Glob(filepath.Join(gitDir, "kandev-turn-changes-index-*"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || now.Sub(info.ModTime()) <= turnCheckpointLockStale {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func createTurnCheckpointIndex(gitDir string) (string, error) {
	file, err := os.CreateTemp(gitDir, "kandev-turn-changes-index-")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func copyTurnCheckpointIndex(source, destination string) (bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		present, retry, err := copyTurnCheckpointIndexAttempt(source, destination)
		if err != nil {
			return false, err
		}
		if !retry {
			return present, nil
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return false, fmt.Errorf("source Git index changed while it was copied")
}

func copyTurnCheckpointIndexAttempt(source, destination string) (bool, bool, error) {
	locked, err := sourceTurnCheckpointIndexLocked(source)
	if err != nil {
		return false, false, err
	}
	if locked {
		return false, true, nil
	}
	before, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !before.Mode().IsRegular() || before.Size() > turnCheckpointIndexLimit {
		return false, false, fmt.Errorf("source Git index is not a bounded regular file")
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return false, false, err
	}
	after, err := os.Stat(source)
	if err != nil {
		return false, true, nil
	}
	locked, err = sourceTurnCheckpointIndexLocked(source)
	if err != nil {
		return false, false, err
	}
	if locked || !sameTurnCheckpointIndex(before, after, len(contents)) {
		return false, true, nil
	}
	if err := os.WriteFile(destination, contents, 0o600); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func sourceTurnCheckpointIndexLocked(source string) (bool, error) {
	_, err := os.Stat(source + ".lock")
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func sameTurnCheckpointIndex(before, after os.FileInfo, bytesRead int) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime()) && int64(bytesRead) == after.Size()
}

func removeTurnCheckpointIndex(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + ".lock")
}
