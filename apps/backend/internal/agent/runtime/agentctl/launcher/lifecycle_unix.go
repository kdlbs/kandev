//go:build !windows

package launcher

import (
	"context"
	"os/exec"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

// installChildLifecycle is a no-op on Unix. The Unix parent-liveness path uses
// the inherited pipe in launcher_pipe_unix.go (the kernel closes the write-end
// when the parent dies, signaling agentctl to shut down).
func (l *Launcher) installChildLifecycle(_ *exec.Cmd) error {
	return nil
}

func (l *Launcher) containOwnedChildren(ctx context.Context, identity processidentity.Identity) error {
	return processidentity.TerminateOwnedSession(ctx, identity, 500*time.Millisecond)
}

// releaseChildLifecycle is a no-op on Unix.
func (l *Launcher) releaseChildLifecycle() {}
