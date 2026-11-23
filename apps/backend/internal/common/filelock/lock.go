package filelock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Acquire locks one target path across processes running as the same user.
func Acquire(ctx context.Context, namespace, target string) (*os.File, error) {
	if ctx == nil {
		return nil, fmt.Errorf("file lock context is required")
	}
	lockPath, err := Path(namespace, target)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := openLockFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockFile(ctx, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
	return file, nil
}

// Release unlocks and closes a file lock.
func Release(file *os.File) error {
	if file == nil {
		return nil
	}
	unlockErr := unlockFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

// DirectoryPath returns the per-user lock directory for a namespace.
func DirectoryPath(namespace string) (string, error) {
	if namespace == "" || strings.ContainsAny(namespace, `/\\`) || namespace == "." || namespace == ".." {
		return "", fmt.Errorf("invalid file lock namespace")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	if !filepath.IsAbs(cacheDir) {
		return "", fmt.Errorf("user cache directory is not absolute")
	}
	return filepath.Join(cacheDir, "kandev", namespace), nil
}

// Path returns the stable lock file path for a target.
func Path(namespace, target string) (string, error) {
	directory, err := DirectoryPath(namespace)
	if err != nil {
		return "", err
	}
	identity, err := Identity(target)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(identity))
	return filepath.Join(directory, hex.EncodeToString(digest[:])+".lock"), nil
}

// Identity resolves existing symlinks while retaining missing target components.
func Identity(target string) (string, error) {
	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("make target path absolute: %w", err)
	}
	current := filepath.Clean(absPath)
	missing := make([]string, 0, 4)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve target ancestor: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(absPath), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func lockFile(ctx context.Context, file *os.File) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked, err := tryLockFile(file)
		if err != nil {
			return err
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
