//go:build windows

// Package proctree contains a started command and every descendant it creates
// so callers can terminate the whole tree on cancellation and after exit.
package proctree

import (
	"os/exec"
	"syscall"

	"github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

// Tree carries the Job Object that contains the suspended child and every
// descendant it creates. Shims such as npx -> node -> CLI mean that killing
// only the direct child would leave grandchildren holding inherited pipes.
type Tree struct {
	job winproc.KillOnCloseJob
}

// Prepare starts the command suspended in a new process group so Capture can
// assign it to a job before it runs. Call before Start.
func Prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

// Capture assigns the suspended command to a kill-on-close job and resumes it.
func Capture(cmd *exec.Cmd) (Tree, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return Tree{}, err
	}
	return Tree{job: job}, nil
}

// Kill closes the job, terminating every process in it. Without a job it
// falls back to the direct child.
func (t Tree) Kill(cmd *exec.Cmd) {
	if t.job.Valid() {
		_ = t.job.Close()
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
