//go:build !windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLauncherLogsSignalAttempts(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	launcher := &Launcher{logger: log}
	const missingPID = 1 << 30

	if err := launcher.gracefulStop(missingPID); err == nil {
		t.Fatal("expected missing process to fail graceful stop")
	}
	launcher.forceKill(missingPID)

	for _, message := range []string{
		"agentctl subprocess SIGTERM requested",
		"agentctl subprocess SIGKILL requested",
	} {
		if !launcherLogsContain(observed, message) {
			t.Fatalf("expected debug log %q, got %#v", message, observed.All())
		}
	}
}

func TestLauncherForceKillSignalsProcessGroup(t *testing.T) {
	tmp := t.TempDir()
	childPIDFile := filepath.Join(tmp, "child.pid")
	cmd := exec.Command("sh", "-c", `sleep 30 & echo $! > "$CHILD_PID_FILE"; wait`)
	cmd.Env = append(os.Environ(), "CHILD_PID_FILE="+childPIDFile)
	cmd.SysProcAttr = buildSysProcAttr()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}
	pgid := cmd.Process.Pid
	var childPID int
	t.Cleanup(func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	waitUntilLauncherTest(t, time.Second, func() bool {
		raw, err := os.ReadFile(childPIDFile)
		if err != nil {
			return false
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			return false
		}
		childPID = pid
		return processExistsForLauncherTest(childPID)
	}, "child process pid was not written or running")

	launcher := &Launcher{logger: newLauncherTestLogger(t)}
	launcher.forceKill(pgid)

	waitUntilLauncherTest(t, 2*time.Second, func() bool {
		return !processExistsForLauncherTest(childPID)
	}, "forceKill did not kill process-group child %d", childPID)
}

func launcherLogsContain(logs *observer.ObservedLogs, message string) bool {
	for _, entry := range logs.All() {
		if entry.Message == message {
			return true
		}
	}
	return false
}

func newLauncherTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func processExistsForLauncherTest(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func waitUntilLauncherTest(t *testing.T, timeout time.Duration, condition func() bool, format string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatalf(format, args...)
}
