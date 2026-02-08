//go:build !windows

package launcher

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
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

// diagnosePID logs detailed info about what is holding the port (best-effort, non-fatal).
func (l *Launcher) diagnosePID() {
	portStr := fmt.Sprintf(":%d", l.port)

	// Try lsof first
	if out, err := exec.Command("lsof", "-i", portStr, "-P", "-n").CombinedOutput(); err == nil {
		output := strings.TrimSpace(string(out))
		if output != "" {
			l.logger.Warn("lsof: process(es) holding port",
				zap.Int("port", l.port),
				zap.String("lsof_output", output))
			return
		}
	}

	// lsof found nothing — check netstat for TIME_WAIT / CLOSE_WAIT sockets
	if out, err := exec.Command("netstat", "-an").CombinedOutput(); err == nil {
		var matches []string
		search := fmt.Sprintf(".%d ", l.port)  // macOS netstat uses dot separator
		search2 := fmt.Sprintf(":%d ", l.port) // Linux netstat uses colon
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, search) || strings.Contains(line, search2) {
				matches = append(matches, strings.TrimSpace(line))
			}
		}
		if len(matches) > 0 {
			l.logger.Warn("netstat: socket(s) on port (likely TIME_WAIT)",
				zap.Int("port", l.port),
				zap.String("netstat_output", strings.Join(matches, "\n")))
			return
		}
	}

	// Test IPv4 vs IPv6 separately to identify which is blocked
	v4Err := tryBind("tcp4", portStr)
	v6Err := tryBind("tcp6", portStr)
	l.logger.Warn("port in use but no process found — checking address families",
		zap.Int("port", l.port),
		zap.NamedError("ipv4_bind_err", v4Err),
		zap.NamedError("ipv6_bind_err", v6Err))
}

// tryBind attempts to bind to the given network/address and returns any error.
func tryBind(network, addr string) error {
	ln, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	_ = ln.Close()
	return nil
}

// tryKillPortHolder tries to find and kill the process holding the port.
// Returns true if it found and signaled a process.
func (l *Launcher) tryKillPortHolder() bool {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", l.port)).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false
	}

	killed := false
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid := 0
		if _, err := fmt.Sscanf(line, "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		if pid == os.Getpid() {
			continue
		}
		l.logger.Warn("killing stale process on port",
			zap.Int("port", l.port),
			zap.Int("pid", pid))
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				l.logger.Debug("SIGTERM failed, trying SIGKILL",
					zap.Int("pid", pid),
					zap.Error(err))
				_ = proc.Kill()
			}
			killed = true
		}
	}
	return killed
}
