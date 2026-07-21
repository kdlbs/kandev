//go:build linux

package utility

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

func TestProbeOpenCodeModelsHonorsRefresh(t *testing.T) {
	tmp := t.TempDir()
	agentPath := filepath.Join(tmp, openCodeCommand)
	writeExecutable(t, agentPath, `#!/bin/sh
if [ "$1" = "models" ] && [ "$2" = "--refresh" ]; then
  echo "opencode/fresh"
else
  echo "opencode/stale"
fi
`)

	cached, err := probeOpenCodeModels(context.Background(), agentPath, tmp, false)
	if err != nil {
		t.Fatalf("probeOpenCodeModels returned error: %v", err)
	}
	if len(cached) != 1 || cached[0].ID != "opencode/stale" {
		t.Fatalf("cached models = %#v, want cached model list", cached)
	}

	refreshed, err := probeOpenCodeModels(context.Background(), agentPath, tmp, true)
	if err != nil {
		t.Fatalf("refresh probeOpenCodeModels returned error: %v", err)
	}
	if len(refreshed) != 1 || refreshed[0].ID != "opencode/fresh" {
		t.Fatalf("refreshed models = %#v, want refreshed model list", refreshed)
	}
}

func TestProbeRefreshesOpenCodeCacheBeforeACPSession(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "models-refreshed")
	agentPath := filepath.Join(tmp, openCodeCommand)
	writeExecutable(t, agentPath, `#!/bin/sh
if [ "$1" = "models" ]; then
  if [ "$2" = "--refresh" ]; then
    touch "$OPENCODE_REFRESH_MARKER"
  fi
  echo "opencode/fresh"
  exit 0
fi

read -r INITIALIZE
INITIALIZE_ID=$(printf '%s' "$INITIALIZE" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$INITIALIZE_ID"

read -r NEW_SESSION
SESSION_ID=$(printf '%s' "$NEW_SESSION" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
MODEL="opencode/stale"
if [ -f "$OPENCODE_REFRESH_MARKER" ]; then
  MODEL="opencode/fresh"
fi
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"test","configOptions":[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"%s","options":[{"value":"%s","name":"%s"}]}]}}\n' "$SESSION_ID" "$MODEL" "$MODEL" "$MODEL"
cat >/dev/null
`)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENCODE_REFRESH_MARKER", marker)

	executor := NewACPInferenceExecutor(zap.NewNop())
	resp, err := executor.Probe(context.Background(), &ProbeRequest{
		AgentID: "opencode-acp",
		Refresh: true,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{openCodeCommand, openCodeACPSubcommand},
			WorkDir: tmp,
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("Probe failed: %s", resp.Error)
	}
	if len(resp.Models) != 1 || resp.Models[0].ID != "opencode/fresh" {
		t.Fatalf("models = %#v, want refreshed ACP model list", resp.Models)
	}
}

func TestWaitForACPProcessGroupExitHonorsContextCancellation(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	setACPCommandProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = killACPProcessGroup(pid)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitForACPProcessGroupExit(ctx, pid, 2*time.Second) {
		t.Fatal("wait unexpectedly reported process group exit")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("wait ignored canceled context; elapsed=%s", elapsed)
	}
}

func TestCleanupACPCommandWaitsAfterRequestContextCancellation(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	setACPCommandProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		_ = killACPProcessGroup(pid)
		_ = cmd.Wait()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	core, observed := observer.New(zapcore.DebugLevel)

	cleanupACPCommand(ctx, cmd, acpCommandLifecycleHandle{}, zap.New(core))
	waited = true

	if !zapLogsContain(observed, "ACP command exited after SIGTERM") {
		t.Fatalf("cleanup did not wait for SIGTERM exit after canceled context; logs=%#v", observed.All())
	}
	if processRunning(pid) {
		t.Fatalf("process %d still running after cleanup", pid)
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
