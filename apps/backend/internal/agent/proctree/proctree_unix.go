//go:build !windows

// Package proctree contains a started command and every descendant it creates
// so callers can terminate the whole tree on cancellation and after exit.
package proctree

import (
	"os/exec"
	"syscall"
)

// Tree is the containment handle returned by Capture.
type Tree struct{}

// Prepare places the command in its own process group. Call before Start.
func Prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Capture completes containment after Start.
func Capture(_ *exec.Cmd) (Tree, error) { return Tree{}, nil }

// Kill terminates the command's process group, including descendants that
// outlived the direct child.
func (Tree) Kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// A negative pid targets the whole process group. If the group is gone (or
	// Setpgid did not take effect) fall back to the direct child.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
