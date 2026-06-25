//go:build linux

package utility

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestProbeCleansUpDescendantProcessOnTimeout(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "child")
	installLeakyMockAgent(t, tmp, marker)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ACP_LEAK_MARKER", marker)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	core, observed := observer.New(zapcore.DebugLevel)
	executor := NewACPInferenceExecutor(zap.New(core))
	resp, err := executor.Probe(ctx, &ProbeRequest{
		AgentID: "mock-agent",
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"mock-agent"},
			WorkDir: tmp,
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if resp.Success {
		t.Fatalf("Probe unexpectedly succeeded")
	}

	pid := readPID(t, marker+".pid")
	t.Cleanup(func() {
		if processRunning(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	waitUntil(t, 2*time.Second, func() bool {
		return !processRunning(pid)
	}, "descendant process %d was still running after Probe returned", pid)

	for _, message := range []string{
		"ACP command process group SIGTERM requested",
		"ACP command process group SIGKILL requested",
	} {
		if !zapLogsContain(observed, message) {
			t.Fatalf("expected debug log %q, got %#v", message, observed.All())
		}
	}
}

func zapLogsContain(logs *observer.ObservedLogs, message string) bool {
	for _, entry := range logs.All() {
		if entry.Message == message {
			return true
		}
	}
	return false
}

func installLeakyMockAgent(t *testing.T, dir, marker string) {
	t.Helper()

	agentPath := filepath.Join(dir, "mock-agent")
	childPath := filepath.Join(dir, "mock-agent-child")
	writeExecutable(t, agentPath, fmt.Sprintf(`#!/bin/sh
"%s" "$ACP_LEAK_MARKER" &
echo "$!" > "$ACP_LEAK_MARKER.pid"
sleep 30
`, childPath))
	writeExecutable(t, childPath, `#!/bin/sh
trap '' TERM INT HUP
touch "$1.ready"
sleep 30
`)

	t.Setenv("ACP_LEAK_MARKER", marker)
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	var raw []byte
	waitUntil(t, 2*time.Second, func() bool {
		var err error
		raw, err = os.ReadFile(path)
		return err == nil
	}, "pid file %s was not written", path)

	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse pid file %s: %v", path, err)
	}
	return pid
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool, format string, args ...any) {
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

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	return !strings.Contains(string(stat), ") Z ")
}
