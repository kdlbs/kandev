//go:build windows

package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// diagnosePID uses netstat to log what is holding the port (best-effort, non-fatal).
func (l *Launcher) diagnosePID() {
	out, err := exec.Command("netstat", "-ano").CombinedOutput()
	if err != nil {
		l.logger.Debug("netstat failed", zap.Error(err))
		return
	}

	portStr := fmt.Sprintf(":%d ", l.port)
	var matches []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, portStr) {
			matches = append(matches, strings.TrimSpace(line))
		}
	}

	if len(matches) == 0 {
		l.logger.Warn("port in use but netstat found no process — likely TIME_WAIT or kernel hold",
			zap.Int("port", l.port))
	} else {
		l.logger.Warn("process(es) holding port",
			zap.Int("port", l.port),
			zap.String("netstat_output", strings.Join(matches, "\n")))
	}
}

// tryKillPortHolder tries to find and kill the process holding the port using netstat + taskkill.
// Returns true if it found and signaled a process.
func (l *Launcher) tryKillPortHolder() bool {
	out, err := exec.Command("netstat", "-ano").CombinedOutput()
	if err != nil {
		return false
	}

	portStr := fmt.Sprintf(":%d ", l.port)
	selfPid := os.Getpid()
	killed := false

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, portStr) || !strings.Contains(line, "LISTENING") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 5 {
			continue
		}
		pid := 0
		if _, err := fmt.Sscanf(fields[len(fields)-1], "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		if pid == selfPid {
			continue
		}
		l.logger.Warn("killing stale process on port",
			zap.Int("port", l.port),
			zap.Int("pid", pid))
		// taskkill /F /PID <pid>
		if killErr := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid)).Run(); killErr != nil {
			l.logger.Debug("taskkill failed", zap.Int("pid", pid), zap.Error(killErr))
		} else {
			killed = true
		}
	}
	return killed
}
