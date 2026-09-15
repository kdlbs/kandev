//go:build windows

// Package winproc keeps the historical agentctl import path for Windows
// process lifecycle helpers. The implementation is shared with platform
// subprocess callers in internal/common/winproc.
package winproc

import (
	"os/exec"

	commonwinproc "github.com/kandev/kandev/internal/common/winproc"
)

type KillOnCloseJob = commonwinproc.KillOnCloseJob

func InstallKillOnCloseJobForSuspendedCommand(cmd *exec.Cmd) (KillOnCloseJob, error) {
	return commonwinproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
}

func InstallKillOnCloseJobForCommand(cmd *exec.Cmd) (KillOnCloseJob, error) {
	return commonwinproc.InstallKillOnCloseJobForCommand(cmd)
}

func InstallKillOnCloseJobForProcess(pid int) (KillOnCloseJob, error) {
	return commonwinproc.InstallKillOnCloseJobForProcess(pid)
}

func InstallKillOnCloseJobForSuspendedProcess(pid int) (KillOnCloseJob, error) {
	return commonwinproc.InstallKillOnCloseJobForSuspendedProcess(pid)
}

func ResumeSuspendedProcess(pid int) error {
	return commonwinproc.ResumeSuspendedProcess(pid)
}

func RunTaskkill(args ...string) error { return commonwinproc.RunTaskkill(args...) }

func IsTaskkillMissing(msg string) bool { return commonwinproc.IsTaskkillMissing(msg) }
