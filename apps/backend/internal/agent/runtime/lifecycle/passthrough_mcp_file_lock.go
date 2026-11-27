package lifecycle

import (
	"context"
	"os"

	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

func acquirePassthroughMCPFileLock(ctx context.Context, path string) (*os.File, error) {
	return passthroughconfig.Acquire(ctx, path)
}
func releasePassthroughMCPFileLock(file *os.File) error { return passthroughconfig.Release(file) }
func passthroughMCPFileLockIdentity(path string) (string, error) {
	return passthroughconfig.Identity(path)
}
