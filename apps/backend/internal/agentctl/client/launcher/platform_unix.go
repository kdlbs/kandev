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

// forceKill sends SIGKILL to the entire process group.
// Since agentctl starts with Setpgid=true, its PID == PGID, so killing -pid
// terminates agentctl and all children that haven't created their own groups
// (including code-server and its Node.js worker processes).
func (l *Launcher) forceKill(pid int) {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		// Fallback: kill the single process if group kill fails
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
