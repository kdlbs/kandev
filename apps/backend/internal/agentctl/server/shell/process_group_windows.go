//go:build windows

package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

func configureShellProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

type shellProcessLifecycleHandle struct {
	job winproc.KillOnCloseJob
}

func installShellProcessLifecycle(cmd *exec.Cmd) (shellProcessLifecycleHandle, error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return shellProcessLifecycleHandle{}, err
	}
	return shellProcessLifecycleHandle{job: job}, nil
}

func reapShellProcessLifecycle(lifecycle shellProcessLifecycleHandle) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return lifecycle.job.TerminateAndWait(ctx)
}

func ownsShellProcessLifecycle(lifecycle shellProcessLifecycleHandle) bool {
	return lifecycle.job.Valid()
}

func killShellProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := runShellTaskkill("/F", "/T", "/PID", fmt.Sprintf("%d", p.Pid)); err != nil {
		if killErr := p.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}

// taskkill /T waits for the requested process tree termination before it
// returns; Windows does not expose Unix-style process-group liveness here.
func shellProcessGroupAlive(_ *os.Process) bool { return false }

func runShellTaskkill(args ...string) error {
	return winproc.RunTaskkill(args...)
}
