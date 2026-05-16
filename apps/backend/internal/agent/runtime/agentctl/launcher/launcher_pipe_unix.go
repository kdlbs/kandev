//go:build !windows

package launcher

import (
	"fmt"
	"os"
	"os/exec"
)

// setupLivenessPipe creates a pipe and configures the command to pass the
// read-end to the child via ExtraFiles (FD 3). The child monitors this pipe;
// when the parent dies the kernel closes the write-end, breaking the pipe and
// signaling the child to shut down. Returns the write-end (caller must keep it
// open) or an error.
func setupLivenessPipe(cmd *exec.Cmd) (*os.File, error) {
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create parent liveness pipe: %w", err)
	}

	cmd.ExtraFiles = []*os.File{pipeRead} // child FD 3
	cmd.Env = append(cmd.Env, "KANDEV_PARENT_PIPE_FD=3")

	return pipeWrite, nil
}

// closePipeOnStartFailure cleans up both pipe ends when cmd.Start fails.
func closePipeOnStartFailure(pipeWrite *os.File, cmd *exec.Cmd) {
	if pipeWrite != nil {
		_ = pipeWrite.Close()
	}
	if len(cmd.ExtraFiles) > 0 {
		_ = cmd.ExtraFiles[0].Close()
	}
}

// closeChildPipeEnd closes the read-end of the pipe in the parent process
// after the child has inherited it.
func closeChildPipeEnd(cmd *exec.Cmd) {
	if len(cmd.ExtraFiles) > 0 {
		_ = cmd.ExtraFiles[0].Close()
	}
}
