package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

func (v *VscodeManager) readProcessStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		v.logger.Debug("code-server", zap.String("line", scanner.Text()))
	}
}

func (v *VscodeManager) monitorProcess(cmd *exec.Cmd, doneCh chan struct{}) {
	defer close(doneCh)
	pid := cmd.Process.Pid
	waitErr := cmd.Wait()
	reapErr := v.ensureProcessGroupReaped(pid)

	v.mu.Lock()
	defer v.mu.Unlock()
	if reapErr != nil {
		v.reapErr = reapErr
		v.logger.Error("code-server process tree was not reaped", zap.Int("pid", pid), zap.Error(reapErr))
	}
	if waitErr != nil && v.status != VscodeStatusStopped {
		v.status = VscodeStatusError
		v.err = waitErr.Error()
		v.logger.Error("code-server exited with error", zap.Int("pid", pid), zap.Error(waitErr))
		return
	}
	v.status = VscodeStatusStopped
	v.logger.Info("code-server exited", zap.Int("pid", pid))
}

// Stop stops the code-server process.
func (v *VscodeManager) Stop(ctx context.Context) error {
	startDone := v.beginStop()
	if err := waitForVscodeStartGeneration(ctx, startDone); err != nil {
		return err
	}
	cmd, doneCh := v.snapshotStoppedProcess(startDone)
	if cmd != nil && cmd.Process != nil {
		return v.stopRunningProcess(ctx, cmd.Process.Pid, doneCh)
	}
	if doneCh != nil {
		return waitForVscodeStartupReap(ctx, doneCh)
	}
	v.logger.Info("code-server stopped")
	return nil
}

func (v *VscodeManager) beginStop() chan struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()
	startDone := v.startDone
	cancelStart := v.cancelStart
	v.cancelStart = nil
	if cancelStart != nil {
		cancelStart()
	}
	if v.afterStartCancel != nil {
		v.afterStartCancel()
	}
	v.logger.Info("stopping code-server")
	v.status = VscodeStatusStopped
	if v.stopCh != nil && !v.stopped {
		close(v.stopCh)
		v.stopped = true
	}
	return startDone
}

func waitForVscodeStartGeneration(ctx context.Context, startDone <-chan struct{}) error {
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			return fmt.Errorf("wait for code-server startup generation: %w", ctx.Err())
		case <-time.After(2 * time.Second):
			return fmt.Errorf("code-server startup generation did not stop after cancellation")
		}
	}
	return nil
}

func (v *VscodeManager) snapshotStoppedProcess(startDone chan struct{}) (*exec.Cmd, <-chan struct{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	cmd := v.cmd
	doneCh := v.doneCh
	if v.startDone == startDone {
		v.startDone = nil
	}
	v.status = VscodeStatusStopped
	return cmd, doneCh
}

func (v *VscodeManager) stopRunningProcess(ctx context.Context, pid int, doneCh <-chan struct{}) error {
	v.logger.Info("stopping code-server process group", zap.Int("pid", pid), zap.Int("pgid", pid))
	logChildProcesses(v.logger, pid)
	v.logger.Debug("code-server process group SIGTERM requested",
		zap.Int("pgid", pid), zap.String("reason", "stop_requested"))
	if err := v.terminateProcessGroup(pid); err != nil {
		v.logger.Warn("failed to send SIGTERM to code-server group", zap.Int("pgid", pid), zap.Error(err))
	}
	if doneCh == nil {
		return v.ensureProcessGroupReaped(pid)
	}
	select {
	case <-doneCh:
		v.logger.Info("code-server stopped gracefully", zap.Int("pid", pid))
		return v.ensureProcessGroupReaped(pid)
	case <-ctx.Done():
		v.logger.Warn("context cancelled during SIGTERM wait, force killing", zap.Int("pgid", pid))
	case <-time.After(5 * time.Second):
		v.logger.Warn("code-server did not exit after SIGTERM, force killing", zap.Int("pgid", pid))
	}
	return v.forceStopRunningProcess(ctx, pid, doneCh)
}

func (v *VscodeManager) forceStopRunningProcess(ctx context.Context, pid int, doneCh <-chan struct{}) error {
	v.logger.Debug("code-server process group SIGKILL requested",
		zap.Int("pgid", pid), zap.String("reason", "grace_expired_or_context_canceled"))
	if err := v.killProcessGroup(pid); err != nil {
		v.logger.Warn("failed to force kill code-server group", zap.Int("pgid", pid), zap.Error(err))
	}
	select {
	case <-doneCh:
		return v.ensureProcessGroupReaped(pid)
	case <-ctx.Done():
		return fmt.Errorf("wait for code-server process reap: %w", ctx.Err())
	case <-time.After(2 * time.Second):
		v.logger.Error("code-server still alive after SIGKILL", zap.Int("pid", pid))
		return fmt.Errorf("code-server process %d was not reaped after force kill", pid)
	}
}

func waitForVscodeStartupReap(ctx context.Context, doneCh <-chan struct{}) error {
	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for code-server startup reap: %w", ctx.Err())
	case <-time.After(2 * time.Second):
		return fmt.Errorf("code-server startup process was not reaped after stop")
	}
}

func (v *VscodeManager) ensureProcessGroupReaped(pid int) error {
	v.mu.Lock()
	lifecycle := v.lifecycle
	v.mu.Unlock()
	if err := reapProcessLifecycle(lifecycle); err != nil {
		return fmt.Errorf("reap code-server process job: %w", err)
	}
	if !v.processGroupAlive(pid) {
		v.clearReapedProcessOwnership(lifecycle)
		return nil
	}
	_ = v.killProcessGroup(pid)
	ctx, cancel := context.WithTimeout(context.Background(), processGroupTerminateGrace)
	defer cancel()
	if v.waitForProcessGroupExit(ctx, pid) {
		v.clearReapedProcessOwnership(lifecycle)
		return nil
	}
	return fmt.Errorf("code-server process group %d remains alive", pid)
}

func (v *VscodeManager) clearReapedProcessOwnership(lifecycle processLifecycleHandle) {
	v.mu.Lock()
	if v.lifecycle == lifecycle {
		v.lifecycle = processLifecycleHandle{}
		v.reapErr = nil
	}
	v.mu.Unlock()
}

func (v *VscodeManager) processGroupAlive(pid int) bool {
	if v.groupAliveFn != nil {
		return v.groupAliveFn(pid)
	}
	return processGroupAlive(pid)
}

func (v *VscodeManager) killProcessGroup(pid int) error {
	if v.killGroupFn != nil {
		return v.killGroupFn(pid)
	}
	return killProcessGroup(pid)
}

func (v *VscodeManager) terminateProcessGroup(pid int) error {
	if v.terminateGroupFn != nil {
		return v.terminateGroupFn(pid)
	}
	return terminateProcessGroup(pid)
}

func (v *VscodeManager) waitForProcessGroupExit(ctx context.Context, pid int) bool {
	if v.waitGroupExitFn != nil {
		return v.waitGroupExitFn(ctx, pid)
	}
	return waitForProcessGroupExit(ctx, pid)
}

func (v *VscodeManager) HasUnreapedOwnership() bool {
	v.mu.Lock()
	startDone := v.startDone
	cmd := v.cmd
	reapErr := v.reapErr
	v.mu.Unlock()
	if startDone != nil {
		select {
		case <-startDone:
		default:
			return true
		}
	}
	return reapErr != nil || (cmd != nil && cmd.Process != nil && v.processGroupAlive(cmd.Process.Pid))
}
