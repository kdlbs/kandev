//go:build !windows

package launcher

import (
	"testing"

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

func launcherLogsContain(logs *observer.ObservedLogs, message string) bool {
	for _, entry := range logs.All() {
		if entry.Message == message {
			return true
		}
	}
	return false
}
