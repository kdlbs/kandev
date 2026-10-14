package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const passthroughMCPFileLockDirectory = "kandev/passthrough-mcp-locks"

// acquirePassthroughMCPFileLock serializes project MCP operations across
// backend processes. The lock lives in the current user's cache directory
// rather than the workspace, so it cannot become user project configuration
// or collide with another operating system user's backend.
func acquirePassthroughMCPFileLock(ctx context.Context, path string) (*os.File, error) {
	if ctx == nil {
		return nil, fmt.Errorf("lock passthrough MCP file: context is required")
	}
	lockPath, err := passthroughMCPFileLockPath(path)
	if err != nil {
		return nil, fmt.Errorf("resolve passthrough MCP lock path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create passthrough MCP lock directory: %w", err)
	}
	file, err := openPassthroughMCPFileLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("open passthrough MCP lock: %w", err)
	}
	if err := lockPassthroughMCPFile(ctx, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock passthrough MCP file: %w", err)
	}
	return file, nil
}

func lockPassthroughMCPFile(ctx context.Context, file *os.File) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked, err := tryLockPassthroughMCPFile(file)
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

func releasePassthroughMCPFileLock(file *os.File) error {
	if file == nil {
		return nil
	}
	unlockErr := unlockPassthroughMCPFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func passthroughMCPFileLockDirectoryPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	if !filepath.IsAbs(cacheDir) {
		return "", fmt.Errorf("user cache directory %q is not absolute", cacheDir)
	}
	return filepath.Join(cacheDir, passthroughMCPFileLockDirectory), nil
}

func passthroughMCPFileLockPath(path string) (string, error) {
	lockDirectory, err := passthroughMCPFileLockDirectoryPath()
	if err != nil {
		return "", err
	}
	identity, err := passthroughMCPFileLockIdentity(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(identity))
	name := hex.EncodeToString(digest[:]) + ".lock"
	return filepath.Join(lockDirectory, name), nil
}

// passthroughMCPFileLockIdentity resolves the deepest existing ancestor of a
// target and appends missing components. This makes a workspace symlink and
// its canonical path share one lock even before the target file exists.
func passthroughMCPFileLockIdentity(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make passthrough MCP lock path absolute: %w", err)
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
			return "", fmt.Errorf("resolve passthrough MCP lock ancestor %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(absPath), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
