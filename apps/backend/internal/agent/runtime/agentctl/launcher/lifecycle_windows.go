//go:build windows

package launcher

import (
	"context"
	"fmt"
	"os/exec"
	"sync/atomic"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"github.com/kandev/kandev/internal/common/processidentity"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// installChildLifecycle binds the agentctl child to a Windows Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE so the kernel terminates the child when
// the launcher (backend) process exits — including unexpected crashes. Without
// this, a crashed parent leaves agentctl.exe alive and holding its control
// port (41001 by default), forcing the user to kill the process by hand or
// reboot before kandev can start again (issue #892).
//
// The handle is intentionally kept open for the lifetime of the parent
// process: the OS closes it as part of normal process teardown on every exit
// path (clean shutdown, panic, signal, hard kill), and that close is exactly
// what releases the kill-on-close interlock. releaseChildLifecycle is provided
// for the rare case where the launcher is restarted within the same process.
func (l *Launcher) installChildLifecycle(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("installChildLifecycle: process not started")
	}

	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return err
	}

	// Defensive: if a previous lifecycle was somehow installed without a
	// matching release (unexpected restart, future code change), close the
	// stale handle instead of silently leaking it. Doing the swap *after* the
	// new job is assigned avoids killing the new agentctl before we've stored
	// its handle — the new job has KILL_ON_JOB_CLOSE so we must hold a
	// reference at all times.
	l.childLifecycleMu.Lock()
	previous := atomic.SwapUintptr(&l.jobHandle, job.RawHandle())
	l.childLifecycleMu.Unlock()
	if previous != 0 {
		if err := windows.CloseHandle(windows.Handle(previous)); err != nil {
			l.logger.Warn("failed to close stale job object handle", zap.Error(err))
		} else {
			l.logger.Warn("closed stale job object handle from previous install")
		}
	}
	l.logger.Info("agentctl bound to kill-on-close job object",
		zap.Int("pid", cmd.Process.Pid))
	return nil
}

// releaseChildLifecycle closes the job-object handle if one was installed.
// Safe to call at any point after the child was started: on normal shutdown
// paths the child has already exited and this is a plain handle close; in
// the force-kill timeout branch of Stop the child may still be alive, and the
// handle close acts as a backstop — KILL_ON_JOB_CLOSE terminates every
// remaining process in the job. Also safe to call when no handle was set or
// after a previous release.
func (l *Launcher) releaseChildLifecycle() {
	l.childLifecycleMu.Lock()
	defer l.childLifecycleMu.Unlock()
	handle := atomic.SwapUintptr(&l.jobHandle, 0)
	if handle == 0 {
		return
	}
	if err := windows.CloseHandle(windows.Handle(handle)); err != nil {
		l.logger.Warn("failed to close job object handle", zap.Error(err))
	}
}

func (l *Launcher) containOwnedChildren(ctx context.Context, identity processidentity.Identity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	l.childLifecycleMu.Lock()
	handle := atomic.LoadUintptr(&l.jobHandle)
	if handle == 0 {
		l.childLifecycleMu.Unlock()
		return fmt.Errorf("agentctl Job Object is unavailable: %w", processidentity.ErrUnverifiableIdentity)
	}
	err := winproc.TerminateJobAndWaitHandle(ctx, handle)
	l.childLifecycleMu.Unlock()
	// Closing this owner-held handle is the kernel backstop even when the
	// accounting wait fails or reaches its deadline.
	l.releaseChildLifecycle()
	return err
}
