//go:build !windows

package launcher

import "os/exec"

// installChildLifecycle is a no-op on Unix. The Unix parent-liveness path uses
// the inherited pipe in launcher_pipe_unix.go (the kernel closes the write-end
// when the parent dies, signaling agentctl to shut down).
func (l *Launcher) installChildLifecycle(_ *exec.Cmd) error {
	return nil
}

// releaseChildLifecycle is a no-op on Unix.
func (l *Launcher) releaseChildLifecycle() {}
