//go:build windows

package launcher

import (
	"os"
	"os/exec"
)

// setupLivenessPipe is a no-op on Windows. ExtraFiles is not supported by
// Go's Windows process creation. The agentctl child handles the missing
// KANDEV_PARENT_PIPE_FD env var gracefully (monitorParentLiveness returns nil).
// On Windows, process cleanup relies on gracefulStop (CTRL_BREAK_EVENT).
func setupLivenessPipe(_ *exec.Cmd) (*os.File, error) {
	return nil, nil
}

// closePipeOnStartFailure is a no-op on Windows (no pipe to clean up).
func closePipeOnStartFailure(_ *os.File, _ *exec.Cmd) {}

// closeChildPipeEnd is a no-op on Windows (no pipe to close).
func closeChildPipeEnd(_ *exec.Cmd) {}
