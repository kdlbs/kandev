package passthroughconfig

import (
	"context"
	"os"
)

func Acquire(ctx context.Context, path string) (*os.File, error) {
	return acquirePassthroughMCPFileLock(ctx, path)
}
func Release(file *os.File) error          { return releasePassthroughMCPFileLock(file) }
func Identity(path string) (string, error) { return passthroughMCPFileLockIdentity(path) }
