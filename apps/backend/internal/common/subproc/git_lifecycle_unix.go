//go:build unix

package subproc

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type gitLifecycleHandle struct {
	pid int
}

func prepareGitLifecycleCommand(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	attr := *cmd.SysProcAttr
	if attr.Setctty || attr.Foreground || attr.Ctty != 0 {
		return errors.New("Git command requests a controlling terminal")
	}
	if attr.Setpgid || attr.Pgid != 0 {
		return errors.New("Git command requests caller-owned process-group attributes")
	}
	// setsid creates a new session and process group whose ID is the child
	// PID. Combining it with Setpgid is rejected by the kernel on several
	// Unix platforms, so the session is the ownership boundary we signal.
	attr.Setsid = true
	attr.Setpgid = false
	attr.Pgid = 0
	cmd.SysProcAttr = &attr
	setGitWaitDelay(cmd)
	return nil
}

func installGitLifecycle(cmd *exec.Cmd) (gitLifecycleHandle, error) {
	return gitLifecycleHandle{pid: cmd.Process.Pid}, nil
}

func cancelGitLifecycle(lifecycle gitLifecycleHandle) error {
	if lifecycle.pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-lifecycle.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill Git process group %d: %w", lifecycle.pid, err)
	}
	return nil
}

func releaseGitLifecycle(gitLifecycleHandle) error { return nil }

func abortStartedGit(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return waitForStartedGit(cmd)
}

func waitForStartedGit(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(gitCleanupLimit):
		return fmt.Errorf("Git process %d was not reaped after startup failure", cmd.Process.Pid)
	}
}
