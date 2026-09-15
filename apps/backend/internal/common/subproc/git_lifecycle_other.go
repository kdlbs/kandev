//go:build !unix && !windows

package subproc

import (
	"fmt"
	"os/exec"
	"time"
)

type gitLifecycleHandle struct{}

func prepareGitLifecycleCommand(cmd *exec.Cmd) error {
	setGitWaitDelay(cmd)
	return nil
}

func installGitLifecycle(*exec.Cmd) (gitLifecycleHandle, error) {
	return gitLifecycleHandle{}, nil
}

func cancelGitLifecycle(gitLifecycleHandle) error { return nil }

func releaseGitLifecycle(gitLifecycleHandle) error { return nil }

func abortStartedGit(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(gitCleanupLimit):
		return fmt.Errorf("Git process was not reaped after startup failure")
	}
}
