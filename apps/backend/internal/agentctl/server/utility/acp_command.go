package utility

import (
	"os"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

func configureACPCommand(cmd *exec.Cmd, log *zap.Logger) {
	log = acpCommandLogger(log)
	setACPCommandProcAttr(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		log.Debug("ACP command process group SIGTERM requested",
			zap.Int("pgid", cmd.Process.Pid),
			zap.String("reason", "context_cancel"))
		if err := terminateACPProcessGroup(cmd.Process.Pid); err != nil {
			if acpProcessGroupMissing(err) {
				return os.ErrProcessDone
			}
			log.Debug("ACP command process group SIGTERM failed",
				zap.Int("pgid", cmd.Process.Pid),
				zap.String("reason", "context_cancel"),
				zap.Error(err))
			return err
		}
		return nil
	}
}

func cleanupACPCommand(cmd *exec.Cmd, log *zap.Logger) {
	log = acpCommandLogger(log)
	if cmd == nil {
		log.Debug("ACP command cleanup skipped", zap.String("reason", "nil_command"))
		return
	}
	if cmd.Process == nil {
		log.Debug("ACP command cleanup skipped", zap.String("reason", "nil_process"))
		return
	}
	pid := cmd.Process.Pid
	log.Debug("ACP command cleanup requested",
		zap.Int("pid", pid),
		zap.String("path", cmd.Path),
		zap.Strings("args", cmd.Args))
	log.Debug("ACP command process group SIGTERM requested",
		zap.Int("pgid", pid),
		zap.String("reason", "cleanup"))
	if err := terminateACPProcessGroup(pid); err != nil && !acpProcessGroupMissing(err) {
		log.Debug("ACP command process group SIGTERM failed",
			zap.Int("pgid", pid),
			zap.String("reason", "cleanup"),
			zap.Error(err))
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	if waitForACPCommand(waitCh, acpCommandTerminateGrace) {
		log.Debug("ACP command exited after SIGTERM", zap.Int("pid", pid))
		reapACPProcessGroup(pid, log)
		return
	}

	log.Debug("ACP command process group SIGKILL requested",
		zap.Int("pgid", pid),
		zap.String("reason", "cleanup_timeout"))
	if err := killACPProcessGroup(pid); err != nil && !acpProcessGroupMissing(err) {
		log.Debug("ACP command process group SIGKILL failed; killing process",
			zap.Int("pgid", pid),
			zap.Int("pid", pid),
			zap.Error(err))
		log.Debug("ACP command process SIGKILL requested",
			zap.Int("pid", pid),
			zap.String("reason", "process_group_kill_failed"))
		_ = cmd.Process.Kill()
	}
	_ = waitForACPCommand(waitCh, acpCommandForceKillGrace)
	reapACPProcessGroup(pid, log)
}

func waitForACPCommand(waitCh <-chan error, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waitCh:
		return true
	case <-timer.C:
		return false
	}
}

func reapACPProcessGroup(pid int, log *zap.Logger) {
	log = acpCommandLogger(log)
	if !acpProcessGroupAlive(pid) {
		return
	}
	log.Debug("ACP command process group still alive; SIGTERM requested",
		zap.Int("pgid", pid),
		zap.String("reason", "reap_descendants"))
	if err := terminateACPProcessGroup(pid); err != nil && !acpProcessGroupMissing(err) {
		log.Debug("ACP command process group SIGTERM failed",
			zap.Int("pgid", pid),
			zap.String("reason", "reap_descendants"),
			zap.Error(err))
	}
	waitForACPProcessGroupExit(pid, acpCommandTerminateGrace)
	if !acpProcessGroupAlive(pid) {
		return
	}
	log.Debug("ACP command process group SIGKILL requested",
		zap.Int("pgid", pid),
		zap.String("reason", "reap_descendants_timeout"))
	if err := killACPProcessGroup(pid); err != nil && !acpProcessGroupMissing(err) {
		log.Debug("ACP command process group SIGKILL failed",
			zap.Int("pgid", pid),
			zap.String("reason", "reap_descendants_timeout"),
			zap.Error(err))
	}
	waitForACPProcessGroupExit(pid, acpCommandForceKillGrace)
}

func acpCommandLogger(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}
	return log
}

func waitForACPProcessGroupExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !acpProcessGroupAlive(pid) {
			return
		}
		time.Sleep(acpCommandPollInterval)
	}
}
