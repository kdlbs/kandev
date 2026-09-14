//go:build windows

package subproc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	commonwinproc "github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

type gitLifecycleHandle struct {
	job commonwinproc.KillOnCloseJob
}

func prepareGitLifecycleCommand(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	attr := *cmd.SysProcAttr
	if attr.CreationFlags&windows.CREATE_NEW_CONSOLE != 0 {
		return errors.New("Git command requests a new controlling console")
	}
	attr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED
	cmd.SysProcAttr = &attr
	setGitWaitDelay(cmd)
	return nil
}

func installGitLifecycle(cmd *exec.Cmd) (gitLifecycleHandle, error) {
	job, err := commonwinproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return gitLifecycleHandle{}, err
	}
	return gitLifecycleHandle{job: job}, nil
}

func cancelGitLifecycle(lifecycle gitLifecycleHandle) error {
	if !lifecycle.job.Valid() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitCleanupLimit)
	defer cancel()
	return lifecycle.job.TerminateAndWait(ctx)
}

func releaseGitLifecycle(lifecycle gitLifecycleHandle) error {
	if !lifecycle.job.Valid() {
		return nil
	}
	return lifecycle.job.Close()
}

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
