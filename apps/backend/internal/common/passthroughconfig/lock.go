package passthroughconfig

import (
	"context"
	"os"

	"github.com/kandev/kandev/internal/common/filelock"
)

const passthroughMCPFileLockDirectory = "kandev/passthrough-mcp-locks"

// acquirePassthroughMCPFileLock serializes project MCP operations across
// backend processes. The lock lives in the current user's cache directory
// rather than the workspace, so it cannot become user project configuration
// or collide with another operating system user's backend.
func acquirePassthroughMCPFileLock(ctx context.Context, path string) (*os.File, error) {
	return filelock.Acquire(ctx, "passthrough-mcp-locks", path)
}

func releasePassthroughMCPFileLock(file *os.File) error {
	if file == nil {
		return nil
	}
	return filelock.Release(file)
}

func passthroughMCPFileLockDirectoryPath() (string, error) {
	return filelock.DirectoryPath("passthrough-mcp-locks")
}

func passthroughMCPFileLockPath(path string) (string, error) {
	return filelock.Path("passthrough-mcp-locks", path)
}

// passthroughMCPFileLockIdentity resolves the deepest existing ancestor of a
// target and appends missing components. This makes a workspace symlink and
// its canonical path share one lock even before the target file exists.
func passthroughMCPFileLockIdentity(path string) (string, error) {
	return filelock.Identity(path)
}
