package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPassthroughMCPFileLockDirectoryUsesUserCache(t *testing.T) {
	cacheDir := t.TempDir()
	expected := filepath.Join(cacheDir, passthroughMCPFileLockDirectory)
	setPassthroughMCPFileLockCacheDir(t, cacheDir)
	switch runtime.GOOS {
	case "plan9":
		expected = filepath.Join(cacheDir, "lib", "cache", passthroughMCPFileLockDirectory)
	case "darwin", "ios":
		expected = filepath.Join(cacheDir, "Library", "Caches", passthroughMCPFileLockDirectory)
	}

	got, err := passthroughMCPFileLockDirectoryPath()

	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestPassthroughMCPFileLockWaitHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	lock, err := acquirePassthroughMCPFileLock(context.Background(), path)
	require.NoError(t, err)
	defer func() { require.NoError(t, releasePassthroughMCPFileLock(lock)) }()

	baseContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &passthroughLockWaitContext{Context: baseContext, waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		secondLock, lockErr := acquirePassthroughMCPFileLock(ctx, path)
		if secondLock != nil {
			_ = releasePassthroughMCPFileLock(secondLock)
		}
		result <- lockErr
	}()

	select {
	case <-ctx.waiting:
		cancel()
	case err := <-result:
		t.Fatalf("second lock returned before entering its wait: %v", err)
	case <-time.After(time.Second):
		t.Fatal("second lock did not wait on the held file lock")
	}

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("second lock did not stop after cancellation")
	}
}

type passthroughLockWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *passthroughLockWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestPassthroughMCPFileLockPathUsesCanonicalTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows CI")
	}
	cacheDir := t.TempDir()
	setPassthroughMCPFileLockCacheDir(t, cacheDir)
	root := t.TempDir()
	realWorkspace := filepath.Join(root, "real")
	aliasWorkspace := filepath.Join(root, "alias")
	require.NoError(t, os.MkdirAll(filepath.Join(realWorkspace, ".pi"), 0o700))
	require.NoError(t, os.Symlink(realWorkspace, aliasWorkspace))

	realPath, err := passthroughMCPFileLockPath(filepath.Join(realWorkspace, ".pi", "mcp.json"))
	require.NoError(t, err)
	aliasPath, err := passthroughMCPFileLockPath(filepath.Join(aliasWorkspace, ".pi", "mcp.json"))
	require.NoError(t, err)

	require.Equal(t, realPath, aliasPath,
		"a symlink alias and its canonical workspace must share the lock")
}

func setPassthroughMCPFileLockCacheDir(t *testing.T, cacheDir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", cacheDir)
	case "darwin", "ios":
		t.Setenv("HOME", cacheDir)
	case "plan9":
		t.Setenv("home", cacheDir)
	default:
		t.Setenv("XDG_CACHE_HOME", cacheDir)
	}
}
