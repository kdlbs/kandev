//go:build !windows

package launcher

import (
	"syscall"

	"go.uber.org/zap"
)

// gracefulStop sends SIGTERM to the process for graceful shutdown.
// Falls back to SIGKILL if SIGTERM fails.
func (l *Launcher) gracefulStop(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		l.logger.Warn("failed to send SIGTERM, trying SIGKILL", zap.Error(err))
		_ = syscall.Kill(pid, syscall.SIGKILL)
		return err
	}
	return nil
}

// forceKill sends SIGKILL to the process.
func (l *Launcher) forceKill(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
