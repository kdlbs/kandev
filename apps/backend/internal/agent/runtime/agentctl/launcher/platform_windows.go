//go:build windows

package launcher

import (
	"os"

	"go.uber.org/zap"
)

// gracefulStop on Windows sends an interrupt signal to the process.
// If that fails, it falls back to Kill().
func (l *Launcher) gracefulStop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// os.Interrupt on Windows sends CTRL_BREAK_EVENT
	if err := proc.Signal(os.Interrupt); err != nil {
		l.logger.Warn("failed to send interrupt, trying kill", zap.Error(err))
		_ = proc.Kill()
		return err
	}
	return nil
}

// forceKill terminates the process on Windows.
func (l *Launcher) forceKill(pid int) {
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}
