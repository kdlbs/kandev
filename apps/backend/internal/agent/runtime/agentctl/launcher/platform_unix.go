//go:build !windows

package launcher

import (
	"syscall"

	"go.uber.org/zap"
)

// gracefulStop sends SIGTERM to the process group for graceful shutdown.
// Falls back to SIGKILL if SIGTERM fails.
func (l *Launcher) gracefulStop(pid int) error {
	l.logger.Debug("agentctl subprocess SIGTERM requested", zap.Int("pgid", pid))
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		l.logger.Warn("failed to send SIGTERM, trying SIGKILL", zap.Error(err))
		l.logger.Debug("agentctl subprocess SIGKILL requested",
			zap.Int("pgid", pid),
			zap.String("reason", "sigterm_failed"))
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return err
	}
	return nil
}

// forceKill sends SIGKILL to the agentctl process group.
func (l *Launcher) forceKill(pid int) {
	l.logger.Debug("agentctl subprocess SIGKILL requested",
		zap.Int("pgid", pid),
		zap.String("reason", "force_kill"))
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
